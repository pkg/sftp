//go:build integration
// +build integration

package localfs

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	goCmp "github.com/google/go-cmp/cmp"

	sftp "github.com/pkg/sftp/v2"
	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

var (
	myUID string
	myGID string
)

type Pipe struct {
	io.Reader
	io.WriteCloser
}

func toRemotePath(p string) string {
	p = filepath.ToSlash(p)
	if !path.IsAbs(p) {
		return "/" + p
	}
	return p
}

var srv = &sftp.Server{
	Handler: handler,
}

var cl *sftp.Client

var (
	cwd    string
	clData []byte
	clInfo fs.FileInfo
)

func teeToFile(wr io.WriteCloser, filename string) (io.WriteCloser, io.Closer) {
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}

	prd, pwr := io.Pipe()

	go func() {
		buf := make([]byte, 32*1<<10)

		for {
			n, err := prd.Read(buf)
			if err != nil {
				wr.Close()
				if !errors.Is(err, io.EOF) {
					panic(err)
				}
				return
			}

			if _, err := f.Write(buf[:n]); err != nil {
				panic(err)
			}

			if _, err := wr.Write(buf[:n]); err != nil {
				panic(err)
			}
		}
	}()

	return pwr, f
}

type delayedWrite struct {
	c <-chan time.Time
	b []byte
}

type delayedWriter struct {
	delay time.Duration
	ch    chan *delayedWrite

	mu      sync.Mutex
	closing chan struct{}
	closed  chan struct{}
}

func newDelayedWriter(wr io.WriteCloser, delay time.Duration) io.WriteCloser {
	dw := &delayedWriter{
		delay: delay,
		ch:    make(chan *delayedWrite, 128),

		closing: make(chan struct{}),
		closed:  make(chan struct{}),
	}

	go func() {
		defer func() {
			wr.Close()
			close(dw.closed)
		}()

		for msg := range dw.ch {
			select {
			case <-msg.c:
				n, err := wr.Write(msg.b)

				if n < len(msg.b) {
					err = cmp.Or(err, io.ErrShortWrite)
				}

				if err != nil {
					panic(fmt.Sprintf("delayedWriter: write error: %v", err))
				}

			case <-dw.closing:
				for range dw.ch {
					// Drain and throw away all outstanding writes.
				}

				return
			}
		}
	}()

	return dw
}

func (dw *delayedWriter) Write(b []byte) (int, error) {
	write := &delayedWrite{
		c: time.After(dw.delay),
		b: slices.Clone(b),
	}

	select {
	case <-dw.closing:
		return 0, fs.ErrClosed
	case dw.ch <- write:
		// No need to queue up a background send.
	default:
		go func() {
			select {
			case <-dw.closing:
			case dw.ch <- write:
			}
		}()
	}

	return len(b), nil
}

func (dw *delayedWriter) Close() error {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	select {
	case <-dw.closing:
	default:
		close(dw.closing)
		close(dw.ch)
	}

	<-dw.closed // wait for writer to finish up.

	return nil
}

var (
	logOutbound    = flag.Bool("log-outbound", false, "if set client.log and server.log records all outbound bytes")
	testServerImpl = flag.Bool("testserver", true, "perform tests against the sftp package server instance")
	debugLevel     = flag.String("l", "QUIET", "sftp-server debug level")
	delay          = flag.Duration("delay", 0, "the amount of delay in the client writer")
)

func TestMain(m *testing.M) {
	flag.Parse()

	var err error
	switch {
	case *testServerImpl:
		err = withServerImpl(m)
	default:
		err = withOpenSSHImpl(m)
	}

	if err != nil {
		fmt.Fprintln(os.Stdout, "FAIL:", err)
		os.Exit(1)
	}
}

func withOpenSSHImpl(m *testing.M) error {
	sftpServerLocations := []string{
		"/usr/libexec/ssh/sftp-server",
		"/usr/libexec/sftp-server",
		"/usr/lib/openssh/sftp-server",
		"/usr/lib/ssh/sftp-server",
		`C:\Program Files\Git\usr\lib\ssh\sftp-server.exe`,
	}

	sftpServer, _ := exec.LookPath("sftp-server")
	if sftpServer == "" {
		for _, loc := range sftpServerLocations {
			if _, err := os.Stat(loc); err == nil {
				sftpServer = loc
				break
			}
		}

		if sftpServer == "" {
			return fmt.Errorf("could not find sftp-server")
		}
	}

	cmd := exec.Command(sftpServer, "-e", "-l", *debugLevel) // log to stderr

	cmd.Stderr = os.Stdout

	pw, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	pr, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start sftp-server: %w", err)
	}

	return runClient(m, pr, pw)
}

func withServerImpl(m *testing.M) error {
	srvConn, clConn := new(Pipe), new(Pipe)

	var err error
	srvConn.Reader, clConn.WriteCloser, err = os.Pipe()
	if err != nil {
		return err
	}
	clConn.Reader, srvConn.WriteCloser, err = os.Pipe()
	if err != nil {
		return err
	}

	if *logOutbound {
		var clCloser io.Closer
		clConn.WriteCloser, clCloser = teeToFile(clConn.WriteCloser, "client.log")
		defer clCloser.Close()

		var srvCloser io.Closer
		srvConn.WriteCloser, srvCloser = teeToFile(srvConn.WriteCloser, "server.log")
		defer srvCloser.Close()
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		if err := srv.Serve(srvConn); err != nil {
			panic(err)
		}
	}()

	defer func() {
		if err := srv.GracefulStop(); err != nil {
			panic(err)
		}

		wg.Wait()
	}()

	return runClient(m, clConn.Reader, clConn.WriteCloser)
}

func runClient(m *testing.M, rd io.Reader, wr io.WriteCloser) error {
	if *delay != 0 {
		wr = newDelayedWriter(wr, *delay)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if runtime.GOOS != "windows" {
		me, err := user.Current()
		if err != nil {
			return err
		}

		myUID = me.Uid
		myGID = me.Gid
	}

	var err error

	cl, err = sftp.NewClientPipe(ctx, rd, wr)
	if err != nil {
		return err
	}
	defer cl.Close()

	defer func() {
		cl.ReportPoolMetrics(os.Stdout)
	}()

	cwd, err = cl.RealPath(".")
	if err != nil {
		return err
	}

	// Read "client.go" to test against various ways to read a file.
	clData, err = os.ReadFile("../client.go")
	if err != nil {
		return err
	}

	clInfo, err = os.Stat("../client.go")
	if err != nil {
		return err
	}

	retCode := m.Run()
	if retCode != 0 {
		return fmt.Errorf("ret code: %d", retCode)
	}
	return nil
}

func TestMkdir(t *testing.T) {
	dir := t.TempDir()

	mkdirTarget := filepath.Join(dir, "test-mkdir")

	t.Log("mkdir:", mkdirTarget)

	if err := cl.Mkdir(toRemotePath(mkdirTarget), 0755); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Stat(mkdirTarget)
	if err != nil {
		t.Fatal(err)
	}

	if !fi.IsDir() {
		t.Errorf("expected %q to be a directory", mkdirTarget)
	}

	if err := cl.Mkdir(toRemotePath(mkdirTarget), 0755); err == nil {
		t.Errorf("expected to error")
	}

	if err := cl.Remove(toRemotePath(mkdirTarget)); err != nil {
		t.Fatal(err)
	}

	_, err = os.Stat(mkdirTarget)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected %q to not exist", mkdirTarget)
	}

	if err := cl.Remove(toRemotePath(mkdirTarget)); err == nil {
		t.Errorf("expected to error")
	}
}

func TestOpenReadWrite(t *testing.T) {
	dir := t.TempDir()

	rdwrTarget := filepath.Join(dir, "test-rdwr")

	t.Log("openfile:", rdwrTarget)

	f, err := cl.OpenFile(toRemotePath(rdwrTarget), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		t.Log("close:", rdwrTarget)

		err := f.Close()
		if err != nil && !errors.Is(err, fs.ErrClosed) {
			t.Error(err)
		}
	}()

	n, err := f.Write([]byte("foo\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("Write() = %d, expected 5", n)
	}

	if _, err := f.Seek(0, os.SEEK_SET); err != nil {
		t.Fatal(err)
	}

	data := make([]byte, 42)

	n, err = f.Read(data)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	data = data[:n]

	if n != 4 {
		t.Errorf("Read() = %d, expected 4", n)
	}

	if string(data) != "foo\n" {
		t.Errorf("Read() = %#v, but expected %#v", string(data), "foo\n")
	}

	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	sys := fi.Sys()

	attrs, ok := sys.(*sshfx.Attributes)
	if !ok {
		t.Fatalf("Stat() = %T, wrong type", sys)
	}

	if attrs.Size != 4 {
		t.Errorf("Stat().Size = %d, but expected 4", attrs.Size)
	}

	if got := fmt.Sprint(attrs.UID); myUID != "" && got != myUID {
		t.Errorf("Stat().UID = %s, but expected %s", got, myUID)
	}

	if got := fmt.Sprint(attrs.GID); myGID != "" && got != myGID {
		t.Errorf("Stat().GID = %s, but expected %s", got, myGID)
	}

	permExpect := sshfx.FileMode(0644)
	if runtime.GOOS == "windows" {
		permExpect = 0666
	}

	if expect := sshfx.ModeRegular | permExpect; attrs.Permissions != expect {
		t.Errorf("Stat().Permissions = %#o, but expected %#o", attrs.Permissions, expect)
	}

	if attrs.ATime == 0 {
		t.Errorf("Stat().ATime shouldn’t be zero")
	}

	if attrs.MTime == 0 {
		t.Errorf("Stat().MTime shouldn’t be zero")
	}

	switch runtime.GOOS {
	case "windows":
		if err := f.Close(); err != nil {
			t.Error(err)
		}

		fallthrough

	default:
		// outside of windows you can delete an open file.
		if err := cl.Remove(rdwrTarget); err != nil {
			t.Error(err)
		}
	}

	if _, err := os.Stat(rdwrTarget); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected %q to not exist", rdwrTarget)
	}
}

func TestRename(t *testing.T) {
	dir := t.TempDir()

	renFrom := filepath.Join(dir, "test-ren-from")
	renTo := filepath.Join(dir, "test-ren-to")

	t.Log(renFrom, "->", renTo)

	if err := cl.WriteFile(renFrom, []byte("foo"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(renFrom); err != nil {
		t.Fatal(err)
	}

	if err := cl.Rename(renFrom, renTo); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(renFrom); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected %q to not exist", renFrom)
	}

	if _, err := os.Stat(renTo); err != nil {
		t.Error(err)
	}
}

func TestReadDirRoot(t *testing.T) {
	fis, err := cl.ReadDir("/")
	if err != nil {
		t.Fatal(err)
	}

	if len(fis) == 0 {
		t.Errorf("ReadDir() []fs.FileInfo shouldn’t be empty")
	}

	for _, fi := range fis {
		switch fi := fi.(type) {
		case *sshfx.NameEntry:
			t.Log(fi.Longname)
		case fs.FileInfo:
			t.Log(sftp.FormatLongname(fi, handler))
		}
	}
}

func TestCWD(t *testing.T) {
	t.Log("readdir:", cwd)

	fis, err := cl.ReadDir(cwd)
	if err != nil {
		t.Fatal(err)
	}

	for _, fi := range fis {
		switch fi := fi.(type) {
		case *sshfx.NameEntry:
			t.Log(fi.Longname)
		case fs.FileInfo:
			t.Log(sftp.FormatLongname(fi, handler))
		}
	}
}

func TestOneBigRead(t *testing.T) {
	f, err := cl.Open("../client.go")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		t.Log("close: ../client.go")

		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()

	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	b := make([]byte, fi.Size()*1024)

	n, err := f.Read(b)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	b = b[:n]

	if !compareClientData(t, "f.Read", "read", b, n) {
		return
	}
}

func TestReadAll(t *testing.T) {
	f, err := cl.Open("../client.go")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		t.Log("close: ../client.go")

		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()

	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}

	if !compareClientData(t, "f.Read", "read", b, len(b)) {
		return
	}
}

func TestWriteTo(t *testing.T) {
	f, err := cl.Open("../client.go")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		t.Log("close: ../client.go")

		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()

	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	buf.Grow(int(fi.Size() + 1024))

	n, err := f.WriteTo(buf)
	if err != nil {
		t.Fatal(err)
	}

	if !compareClientData(t, "f.WriteTo", "writeto", buf.Bytes(), n) {
		return
	}
}

func TestCopy(t *testing.T) {
	f, err := cl.Open("../client.go")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		t.Log("close: ../client.go")

		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()

	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	buf := new(bytes.Buffer)
	buf.Grow(int(fi.Size() + 1024))

	n, err := io.Copy(buf, f)
	if err != nil {
		t.Fatal(err)
	}

	if !compareClientData(t, "io.Copy", "copy", buf.Bytes(), n) {
		return
	}
}

func TestReadFile(t *testing.T) {
	b, err := cl.ReadFile("../client.go")
	if err != nil {
		t.Fatal(err)
	}

	if !compareClientData(t, "cl.ReadFile", "readfile", b, len(b)) {
		return
	}
}

func compareClientData[SZ int | int64](t *testing.T, op, logfile string, b []byte, n SZ) bool {
	t.Helper()

	if int64(n) != clInfo.Size() {
		t.Errorf("%s() = %d bytes, expected %d", op, n, clInfo.Size())
	}

	if diff := goCmp.Diff(clData, b); diff != "" {
		t.Error("os.ReadFile differs from", op)

		if testing.Verbose() {
			t.Error(diff)
		}

		os.WriteFile(fmt.Sprintf("client-%s.out", logfile), b, 0644)

		return false
	}

	return true
}

func TestReadFrom(t *testing.T) {
	dir := t.TempDir()

	readFromTarget := filepath.Join(dir, "client-readfrom.go")

	f, err := cl.Create(toRemotePath(readFromTarget))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		t.Log("close: client-readfrom.go")

		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()

	n, err := f.ReadFrom(bytes.NewReader(clData))
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(readFromTarget)
	if err != nil {
		t.Fatal(err)
	}

	if int64(len(b)) != n {
		t.Errorf("ReadFile() = %d, but ReadFrom() = %d", len(b), n)
	}

	if !compareClientData(t, "f.ReadFrom", "readfrom", b, n) {
		return
	}
}

func TestStatVFS(t *testing.T) {
	if !*testServerImpl {
		t.Skip("not testing against localfs server implementation")
	}

	if _, ok := any(handler).(sftp.StatVFSServerHandler); !ok {
		t.Skip("handler does not implement statvfs")
	}

	dir := t.TempDir()

	targetNotExist := filepath.Join(dir, "statvfs-does-not-exist")

	_, err := cl.StatVFS(toRemotePath(targetNotExist))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unexpected error, got %v, should be fs.ErrNotFound", err)
	}

	resp, err := cl.StatVFS(toRemotePath(dir))
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("%+v", resp)
}

var benchBuf []byte

func benchHelperWriteTo(b *testing.B, length int) {
	b.Helper()

	dir := b.TempDir()

	target := filepath.Join(dir, "bench-writeto")
	remote := toRemotePath(target)

	if err := os.WriteFile(target, nil, 0644); err != nil {
		b.Fatal(err)
	}
	defer os.Remove(remote)

	if err := os.Truncate(target, int64(length)); err != nil {
		b.Fatal(err)
	}

	buf := bytes.NewBuffer(benchBuf)
	if grow := length - buf.Cap(); grow > 0 {
		buf.Grow(grow)
	}
	defer func() {
		benchBuf = buf.Bytes()
	}()

	b.ResetTimer()
	b.SetBytes(int64(length))

	for range b.N {
		buf.Reset()

		f, err := cl.Open(remote)
		if err != nil {
			b.Fatal(err)
		}

		n, err := f.WriteTo(buf)
		if err != nil {
			b.Fatal(err)
		}

		if n != int64(length) {
			b.Fatal(io.ErrShortWrite)
		}

		f.Close()

		if buf.Len() != length {
			b.Fatalf("wrong buffer size: got %d, want %d", buf.Len(), length)
		}
	}
}

func BenchmarkWriteTo1KiB(b *testing.B) {
	benchHelperWriteTo(b, 1<<10)
}

func BenchmarkWriteTo1MiB(b *testing.B) {
	benchHelperWriteTo(b, 1<<20)
}

func BenchmarkWriteTo4MiB(b *testing.B) {
	benchHelperWriteTo(b, 1<<22)
}

func BenchmarkWriteTo10MiB(b *testing.B) {
	benchHelperWriteTo(b, 10*(1<<20)+123)
}

func BenchmarkWriteTo64MiB(b *testing.B) {
	benchHelperWriteTo(b, 1<<26)
}

func benchHelperReadFrom(b *testing.B, length int) {
	b.Helper()

	dir := b.TempDir()

	target := filepath.Join(dir, "bench-readfrom")
	remote := toRemotePath(target)

	if err := os.WriteFile(target, nil, 0644); err != nil {
		b.Fatal(err)
	}
	defer os.Remove(remote)

	data := benchBuf[:cap(benchBuf)]
	if len(data) < length {
		data = make([]byte, length)
	}
	data = data[:length]
	defer func() {
		benchBuf = data
	}()

	buf := new(bytes.Reader)

	b.ResetTimer()
	b.SetBytes(int64(length))

	for range b.N {
		buf.Reset(data)

		f, err := cl.Create(remote)
		if err != nil {
			b.Fatal(err)
		}

		n, err := f.ReadFrom(buf)
		if err != nil {
			b.Fatal(err)
		}

		if n != int64(length) {
			b.Fatal(io.ErrShortWrite)
		}

		f.Close()

		if buf.Len() != 0 {
			b.Fatalf("wrong buffer size: got %d, want %d", buf.Len(), 0)
		}
	}
}

func BenchmarkReadFrom1KiB(b *testing.B) {
	benchHelperReadFrom(b, 1<<10)
}

func BenchmarkReadFrom1MiB(b *testing.B) {
	benchHelperReadFrom(b, 1<<20)
}

func BenchmarkReadFrom4MiB(b *testing.B) {
	benchHelperReadFrom(b, 1<<22)
}

func BenchmarkReadFrom10MiB(b *testing.B) {
	benchHelperReadFrom(b, 10*(1<<20)+123)
}

func BenchmarkReadFrom64MiB(b *testing.B) {
	benchHelperReadFrom(b, 1<<26)
}
