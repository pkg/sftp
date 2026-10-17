//go:build integration && !windows

package sftp

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"testing/quick"
	"time"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
	"github.com/pkg/sftp/v2/internal/sync"
)

const (
	kibi = 1024
	mebi = 1024 * 1024
)

const (
	ReadOnly                = true
	ReadWrite               = false
	NoDelay   time.Duration = 0
)

const debuglevel = "ERROR" // set to "DEBUG" for debugging

var testSftp *string

func TestMain(m *testing.M) {
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
			fmt.Fprintln(os.Stdout, "FAIL: could not find sftp-server")
			os.Exit(1)
		}
	}

	testSftp = flag.String("sftp", sftpServer, "location of the sftp server binary")
	flag.Parse()

	os.Exit(m.Run())
}

type delayedWrite struct {
	t time.Time
	b []byte
}

// delayedWriter wraps a writer and artificially delays the write.
// This is meant to mimic connections with various latencies.
// Errors returned from the underlying writer will panic so this should only be used over reliable connections.
type delayedWriter struct {
	ch      chan delayedWrite
	closing chan struct{}

	wg     sync.WaitGroup
	closed <-chan struct{}
}

func newDelayedWriter(t testing.TB, wr io.WriteCloser, delay time.Duration) *delayedWriter {
	closed := make(chan struct{})

	dw := &delayedWriter{
		ch:      make(chan delayedWrite, 128),
		closing: make(chan struct{}),
		closed:  closed,
	}

	ctx := t.Context()

	go func() {
		defer close(closed)

		defer wr.Close()

		for write := range dw.ch {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(write.t.Add(delay))):
			}

			n, err := wr.Write(write.b)
			if err != nil {
				panic(err)
			}

			if n < len(write.b) {
				panic(io.ErrShortWrite)
			}
		}
	}()

	return dw
}

func (dw *delayedWriter) Write(b []byte) (int, error) {
	select {
	case <-dw.closing:
		return 0, io.ErrClosedPipe
	default:
	}

	dw.wg.Add(1)
	defer dw.wg.Done()

	dw.ch <- delayedWrite{
		t: time.Now(),
		b: slices.Clone(b),
	}

	return len(b), nil
}

func (dw *delayedWriter) Close() error {
	close(dw.closing)

	dw.wg.Wait() // wait for any outstanding blocking writes

	close(dw.ch)

	<-dw.closed // wait for writer goroutine to finish.

	return nil
}

func testClient(t testing.TB, readonly bool, delay time.Duration, opts ...ClientOption) (*Client, *exec.Cmd) {
	args := []string{
		"-e",
		"-l", debuglevel,
	}
	if readonly {
		args = append(args, "-R")
	}

	cmd := exec.Command(*testSftp, args...)

	cmd.Stderr = os.Stdout
	pw, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	if delay > 0 {
		pw = newDelayedWriter(t, pw, delay)
	}

	pr, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Skip("could not start sftp-server process:", err)
	}

	cl, err := newClientPipe(t.Context(), pr, nil, pw, nil, opts)
	if err != nil {
		t.Fatal(err)
	}

	return cl, cmd
}

func sameFile(want, got fs.FileInfo) bool {
	_, wantName := filepath.Split(want.Name())
	_, gotName := filepath.Split(got.Name())
	return wantName == gotName &&
		want.Size() == got.Size()
}

func TestClientLStat(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-lstat")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	want, err := os.Lstat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	got, err := cl.LStat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	if !sameFile(want, got) {
		t.Fatalf("LStat(%q): want %#v, got %#v", f.Name(), want, got)
	}
}

func TestClientLStatIsNotExist(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	name := filepath.Join(t.TempDir(), "sftptest-lstatisnotexist")

	if _, err := cl.LStat(name); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(%v, fs.ErrNotExist) = false, want true", err)
	}
}

func TestClientMkdir(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	sub := filepath.Join(t.TempDir(), "mkdir1")
	if err := cl.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(sub); err != nil {
		t.Fatal(err)
	}
}

func TestClientMkdirAll(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	sub := filepath.Join(t.TempDir(), "mkdir1", "mkdir2", "mkdir3")
	if err := cl.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(sub)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("Expected mkdirall to create dir at: %s", sub)
	}
}

func TestClientOpen(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-open")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	got, err := cl.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientOpenIsNotExist(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	if _, err := cl.Open("/doesnt/exist"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(%v, fs.ErrNotExist) = false, want true", err)
	}
}

func TestClientStatIsNotExist(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	if _, err := cl.Stat("/doesnt/exist"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(%v, fs.ErrNotExist) = false, want true", err)
	}
}

const seekBytes = 128 * 1024

type seek struct {
	offset int64
}

func (s seek) Generate(r *rand.Rand, _ int) reflect.Value {
	s.offset = int64(r.Int31n(seekBytes))
	return reflect.ValueOf(s)
}

func (s seek) set(t *testing.T, r io.ReadSeeker) {
	if _, err := r.Seek(s.offset, io.SeekStart); err != nil {
		t.Fatalf("error while seeking with %+v: %v", s, err)
	}
}

func (s seek) current(t *testing.T, r io.ReadSeeker) {
	const mid = seekBytes / 2

	skip := s.offset / 2
	if s.offset > mid {
		skip = -skip
	}

	if _, err := r.Seek(mid, io.SeekStart); err != nil {
		t.Fatalf("error seeking to midpoint with %+v: %v", s, err)
	}
	if _, err := r.Seek(skip, io.SeekCurrent); err != nil {
		t.Fatalf("error seeking from %d with %+v: %v", mid, s, err)
	}
}

func (s seek) end(t *testing.T, r io.ReadSeeker) {
	if _, err := r.Seek(-s.offset, io.SeekEnd); err != nil {
		t.Fatalf("error seeking from end with %+v: %v", s, err)
	}
}

func readHash(t *testing.T, r io.Reader) (string, int64) {
	h := sha1.New()
	read, err := io.Copy(h, r)
	if err != nil {
		t.Fatal(err)
	}
	return string(h.Sum(nil)), read
}

// writeN writes n bytes of random data to w and returns the
// hash of that data.
func writeN(t *testing.T, w io.Writer, n int64) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	h := sha1.New()

	mw := io.MultiWriter(w, h)

	written, err := io.CopyN(mw, r, n)
	if err != nil {
		t.Fatal(err)
	}
	if written != n {
		t.Fatalf("CopyN(%v): wrote: %v", n, written)
	}
	return string(h.Sum(nil))
}

func TestClientSeek(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	fOS, err := os.CreateTemp(t.TempDir(), "sftptest-seek")
	if err != nil {
		t.Fatal(err)
	}
	defer fOS.Close()

	fSFTP, err := cl.Open(fOS.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer fSFTP.Close()

	writeN(t, fOS, seekBytes)

	if err := quick.CheckEqual(
		func(s seek) (string, int64) { s.set(t, fOS); return readHash(t, fOS) },
		func(s seek) (string, int64) { s.set(t, fSFTP); return readHash(t, fSFTP) },
		nil,
	); err != nil {
		t.Errorf("Seek: expected equal absolute seeks: %v", err)
	}

	if err := quick.CheckEqual(
		func(s seek) (string, int64) { s.current(t, fOS); return readHash(t, fOS) },
		func(s seek) (string, int64) { s.current(t, fSFTP); return readHash(t, fSFTP) },
		nil,
	); err != nil {
		t.Errorf("Seek: expected equal seeks from middle: %v", err)
	}

	if err := quick.CheckEqual(
		func(s seek) (string, int64) { s.end(t, fOS); return readHash(t, fOS) },
		func(s seek) (string, int64) { s.end(t, fSFTP); return readHash(t, fSFTP) },
		nil,
	); err != nil {
		t.Errorf("Seek: expected equal seeks from end: %v", err)
	}
}

func TestClientCreate(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-create")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	f2, err := cl.Create(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
}

func TestClientAppend(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-append")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	f2, err := cl.OpenFile(f.Name(), os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
}

func TestClientCreateFailed(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-createfailed")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	f2, err := cl.Create(f.Name())
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("errors.Is(%v, fs.ErrPermission) = false, want true", err)
	}
	if err == nil {
		f2.Close()
	}
}

func TestClientFileName(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-filename")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	f2, err := cl.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()

	if got, want := f2.Name(), f.Name(); got != want {
		t.Fatalf("Name: got %q want %q", got, want)
	}
}

func TestClientFileStat(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-filestat")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	want, err := os.Lstat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	f2, err := cl.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()

	got, err := f2.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if !sameFile(want, got) {
		t.Fatalf("Stat(%q): want %#v, got %#v", f.Name(), want, got)
	}
}

func TestClientStatLink(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-statlink")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	realName := f.Name()
	linkName := f.Name() + ".softlink"

	// create a symlink that points at sftptest
	if err := os.Symlink(realName, linkName); err != nil {
		t.Fatal(err)
	}

	// compare LStat of links
	wantLStat, err := os.Lstat(linkName)
	if err != nil {
		t.Fatal(err)
	}
	wantStat, err := os.Stat(linkName)
	if err != nil {
		t.Fatal(err)
	}

	gotLStat, err := cl.LStat(linkName)
	if err != nil {
		t.Fatal(err)
	}
	gotStat, err := cl.Stat(linkName)
	if err != nil {
		t.Fatal(err)
	}

	// check that stat is not lstat from os package
	if sameFile(wantLStat, wantStat) {
		t.Fatalf("LStat / Stat(%q): both %#v %#v", f.Name(), wantLStat, wantStat)
	}

	// compare LStat of links
	if !sameFile(wantLStat, gotLStat) {
		t.Fatalf("LStat(%q): want %#v, got %#v", f.Name(), wantLStat, gotLStat)
	}

	// compare Stat of links
	if !sameFile(wantStat, gotStat) {
		t.Fatalf("Stat(%q): want %#v, got %#v", f.Name(), wantStat, gotStat)
	}

	// check that stat is not lstat
	if sameFile(gotLStat, gotStat) {
		t.Fatalf("LStat / Stat(%q): both %#v %#v", f.Name(), gotLStat, gotStat)
	}
}

func TestClientRemove(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-remove")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := cl.Remove(f.Name()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(f.Name()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestClientRemoveAll(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp(t.TempDir(), "sftptest-removeAll")
	if err != nil {
		t.Fatal(err)
	}

	// Create a directory tree
	dir1, err := os.MkdirTemp(tempDir, "foo")
	if err != nil {
		t.Fatal(err)
	}
	dir2, err := os.MkdirTemp(dir1, "bar")
	if err != nil {
		t.Fatal(err)
	}

	// Create some files within the directory tree
	file1 := tempDir + "/file1.txt"
	file2 := dir1 + "/file2.txt"
	file3 := dir2 + "/file3.txt"
	err = os.WriteFile(file1, []byte("File 1"), 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	err = os.WriteFile(file2, []byte("File 2"), 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	err = os.WriteFile(file3, []byte("File 3"), 0644)
	if err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Call the function to delete the files recursively
	err = cl.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("Failed to delete files recursively: %v", err)
	}

	// Check if the directories and files have been deleted
	for _, name := range []string{tempDir, dir1, dir2, file1, file2, file3} {
		if _, err := os.Stat(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s still exists", name)
		}
	}
}

func TestClientRemoveDir(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	dir, err := os.MkdirTemp(t.TempDir(), "sftptest-removedir")
	if err != nil {
		t.Fatal(err)
	}

	if err := cl.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestClientRemoveFailed(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-removefailed")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := cl.Remove(f.Name()); err == nil {
		t.Fatalf("Remove(%v): want: permission denied, got %v", f.Name(), err)
	}
	if _, err := os.Lstat(f.Name()); err != nil {
		t.Fatal(err)
	}
}

func TestClientRename(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "old"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	f2 := filepath.Join(dir, "new")
	if err := cl.Rename(f.Name(), f2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(f.Name()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := os.Lstat(f2); err != nil {
		t.Fatal(err)
	}
}

func TestClientReadLink(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "file"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	f2 := filepath.Join(dir, "symlink")
	if err := os.Symlink(f.Name(), f2); err != nil {
		t.Fatal(err)
	}
	if rl, err := cl.ReadLink(f2); err != nil {
		t.Fatal(err)
	} else if rl != f.Name() {
		t.Fatalf("unexpected link target: %v, not %v", rl, f.Name())
	}
}

func TestClientLink(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	dir := t.TempDir()

	f, err := os.Create(filepath.Join(dir, "file"))
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("linktest")
	_, err = f.Write(data)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}

	f2 := filepath.Join(dir, "link")
	if err := cl.Link(f.Name(), f2); err != nil {
		t.Fatal(err)
	}
	if st2, err := cl.Stat(f2); err != nil {
		t.Fatal(err)
	} else if int(st2.Size()) != len(data) {
		t.Fatalf("unexpected link size: %v, not %v", st2.Size(), len(data))
	}
}

func TestClientSymlink(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "file"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	f2 := filepath.Join(dir, "symlink")
	if err := cl.Symlink(f.Name(), f2); err != nil {
		t.Fatal(err)
	}
	if rl, err := cl.ReadLink(f2); err != nil {
		t.Fatal(err)
	} else if rl != f.Name() {
		t.Fatalf("unexpected link target: %v, not %v", rl, f.Name())
	}
}

func TestClientChmod(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-chmod")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := cl.Chmod(f.Name(), 0531); err != nil {
		t.Fatal(err)
	}
	if stat, err := os.Stat(f.Name()); err != nil {
		t.Fatal(err)
	} else if stat.Mode()&os.ModePerm != 0531 {
		t.Fatalf("invalid perm %o\n", stat.Mode())
	}

	sf, err := cl.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := sf.Chmod(0500); err != nil {
		t.Fatal(err)
	}
	sf.Close()

	stat, err := os.Stat(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode() != 0500 {
		t.Fatalf("invalid perm %o\n", stat.Mode())
	}
}

func TestClientChmodReadonly(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-chmodreadonly")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := cl.Chmod(f.Name(), 0531); err == nil {
		t.Fatal("expected error")
	}
}

func TestClientSetuid(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-setuid")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	const allPerm = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky

	for _, goPerm := range []os.FileMode{
		os.ModeSetuid,
		os.ModeSetgid,
		os.ModeSticky,
		os.ModeSetuid | os.ModeSticky,
	} {
		goPerm |= 0700

		if err := cl.Chmod(f.Name(), goPerm); err != nil {
			t.Fatal(err)
		}

		info, err := cl.Stat(f.Name())
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode() & allPerm; got != goPerm {
			t.Errorf("Chmod(%v): got %v", goPerm, got)
		}

		if err := cl.Chmod(f.Name(), 0700); err != nil { // Reset funny bits.
			t.Fatal(err)
		}
	}
}

func TestClientChown(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	usr, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if usr.Uid != "0" {
		t.Log("must be root to run chown tests")
		t.Skip()
	}

	chownto, err := user.Lookup("daemon") // seems common-ish...
	if err != nil {
		t.Fatal(err)
	}
	toUID, err := strconv.Atoi(chownto.Uid)
	if err != nil {
		t.Fatal(err)
	}
	toGID, err := strconv.Atoi(chownto.Gid)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.CreateTemp(t.TempDir(), "sftptest-chown")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	before, err := exec.Command("ls", "-nl", f.Name()).Output()
	if err != nil {
		t.Fatal(err)
	}
	if err := cl.Chown(f.Name(), toUID, toGID); err != nil {
		t.Fatal(err)
	}
	after, err := exec.Command("ls", "-nl", f.Name()).Output()
	if err != nil {
		t.Fatal(err)
	}

	spaceRegex := regexp.MustCompile(`\s+`)

	beforeWords := spaceRegex.Split(string(before), -1)
	if beforeWords[2] != "0" {
		t.Fatalf("bad previous user? should be root")
	}
	afterWords := spaceRegex.Split(string(after), -1)
	if afterWords[2] != chownto.Uid || afterWords[3] != chownto.Gid {
		t.Fatalf("bad chown: %#v", afterWords)
	}
	t.Logf("before: %v", string(before))
	t.Logf(" after: %v", string(after))
}

func TestClientChownReadonly(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	usr, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if usr.Uid != "0" {
		t.Log("must be root to run chown tests")
		t.Skip()
	}

	chownto, err := user.Lookup("daemon") // seems common-ish...
	if err != nil {
		t.Fatal(err)
	}
	toUID, err := strconv.Atoi(chownto.Uid)
	if err != nil {
		t.Fatal(err)
	}
	toGID, err := strconv.Atoi(chownto.Gid)
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.CreateTemp(t.TempDir(), "sftptest-chownreadonly")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := cl.Chown(f.Name(), toUID, toGID); err == nil {
		t.Fatal("expected error")
	}
}

func TestClientChtimes(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-chtimes")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	atime := time.Date(2013, 2, 23, 13, 24, 35, 0, time.UTC)
	mtime := time.Date(1985, 6, 12, 6, 6, 6, 0, time.UTC)
	if err := cl.Chtimes(f.Name(), atime, mtime); err != nil {
		t.Fatal(err)
	}
	if stat, err := os.Stat(f.Name()); err != nil {
		t.Fatal(err)
	} else if stat.ModTime().Sub(mtime) != 0 {
		t.Fatalf("incorrect mtime: %v vs %v", stat.ModTime(), mtime)
	}
}

func TestClientChtimesReadonly(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-chtimesreadonly")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	atime := time.Date(2013, 2, 23, 13, 24, 35, 0, time.UTC)
	mtime := time.Date(1985, 6, 12, 6, 6, 6, 0, time.UTC)
	if err := cl.Chtimes(f.Name(), atime, mtime); err == nil {
		t.Fatal("expected error")
	}
}

func TestClientTruncate(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-truncate")
	if err != nil {
		t.Fatal(err)
	}
	fname := f.Name()

	if n, err := f.Write([]byte("hello world")); n != 11 || err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := cl.Truncate(fname, 5); err != nil {
		t.Fatal(err)
	}
	if stat, err := os.Stat(fname); err != nil {
		t.Fatal(err)
	} else if stat.Size() != 5 {
		t.Fatalf("unexpected size: %d", stat.Size())
	}
}

func TestClientTruncateReadonly(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp(t.TempDir(), "sftptest-truncreadonly")
	if err != nil {
		t.Fatal(err)
	}
	fname := f.Name()

	if n, err := f.Write([]byte("hello world")); n != 11 || err != nil {
		t.Fatal(err)
	}
	f.Close()

	if err := cl.Truncate(fname, 5); err == nil {
		t.Fatal("expected error")
	}
	if stat, err := os.Stat(fname); err != nil {
		t.Fatal(err)
	} else if stat.Size() != 11 {
		t.Fatalf("unexpected size: %d", stat.Size())
	}
}

// github.com/pkg/sftp/issues/42, abrupt server hangup would result in client hangs.
func TestServerRoughDisconnect(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := cl.Open("/dev/zero")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	go func() {
		time.Sleep(100 * time.Millisecond)
		cmd.Process.Kill()
	}()

	_, err = io.Copy(io.Discard, f)
	if !errors.Is(err, sshfx.StatusConnectionLost) {
		t.Errorf("io.Copy error = %#v, but wanted sshfx.StatusConnectionLost", err)
	}
}

// github.com/pkg/sftp/issues/181, abrupt server hangup would result in client hangs.
// due to broadcastErr filling up the request channel
// this reproduces it about 50% of the time
func TestServerRoughDisconnect2(t *testing.T) {
	cl, cmd := testClient(t, ReadOnly, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := cl.Open("/dev/zero")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	b := make([]byte, 100*32*kibi)

	go func() {
		time.Sleep(100 * time.Millisecond)
		cmd.Process.Kill()
	}()

	for {
		if _, err := f.Read(b); err != nil {
			if !errors.Is(err, sshfx.StatusConnectionLost) {
				t.Errorf("File.Read error = %#v, but wanted sshfx.StatusConnectionLost", err)
			}
			break
		}
	}
}

// github.com/pkg/sftp/issues/234 - abrupt shutdown during ReadFrom hangs client
func TestServerRoughDisconnect3(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	dst, err := cl.OpenFile("/dev/null", OpenFlagReadWrite, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()

	src, err := os.Open("/dev/zero")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	go func() {
		time.Sleep(100 * time.Millisecond)
		cmd.Process.Kill()
	}()

	_, err = io.Copy(dst, src)
	if !errors.Is(err, sshfx.StatusConnectionLost) {
		t.Errorf("io.Copy error = %#v, but wanted sshfx.StatusConnectionLost", err)
	}
}

// github.com/pkg/sftp/issues/234 - also affected Write
func TestServerRoughDisconnect4(t *testing.T) {
	cl, cmd := testClient(t, ReadWrite, NoDelay)
	defer cmd.Wait()
	defer cl.Close()

	dst, err := cl.OpenFile("/dev/null", OpenFlagReadWrite, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()

	src, err := os.Open("/dev/zero")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	b := make([]byte, 200*32*kibi)

	if _, err = src.Read(b); err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		cmd.Process.Kill()
	}()

	for {
		if _, err := dst.Write(b); err != nil {
			if !errors.Is(err, sshfx.StatusConnectionLost) {
				t.Errorf("dst.Write error = %#v, but wanted sshfx.StatusConnectionLost", err)
			}
			break
		}
	}

	_, err = io.Copy(dst, src)
	if !errors.Is(err, sshfx.StatusConnectionLost) {
		t.Errorf("io.Copy error = %#v, but wanted sshfx.StatusConnectionLost", err)
	}
}

func benchmarkRead(b *testing.B, bufsize int, delay time.Duration) {
	size := 10*mebi + 123 // ~10MiB

	cl, cmd := testClient(b, ReadOnly, delay)
	defer cmd.Wait()
	defer cl.Close()

	buf := make([]byte, bufsize)

	b.SetBytes(int64(size))

	for b.Loop() {
		offset := 0

		f, err := cl.Open("/dev/zero")
		if err != nil {
			b.Fatal(err)
		}

		for offset < size {
			remaining := size - offset
			buf := buf[:min(remaining, len(buf))]

			n, err := io.ReadFull(f, buf)
			offset += n

			if err != nil {
				b.Fatalf("read error at %d: %v", offset, err)
			}
		}

		switch {
		case offset < size:
			b.Fatalf("read too few bytes! read: %d, wanted: %d", offset, size)
		case offset > size:
			b.Fatalf("read too many bytes! read: %d, wanted: %d", offset, size)
		}

		f.Close()
	}
}

func BenchmarkRead1k(b *testing.B) {
	benchmarkRead(b, 1*kibi, NoDelay)
}

func BenchmarkRead16k(b *testing.B) {
	benchmarkRead(b, 16*kibi, NoDelay)
}

func BenchmarkRead32k(b *testing.B) {
	benchmarkRead(b, 32*kibi, NoDelay)
}

func BenchmarkRead128k(b *testing.B) {
	benchmarkRead(b, 128*kibi, NoDelay)
}

func BenchmarkRead512k(b *testing.B) {
	benchmarkRead(b, 512*kibi, NoDelay)
}

func BenchmarkRead1MiB(b *testing.B) {
	benchmarkRead(b, mebi, NoDelay)
}

func BenchmarkRead4MiB(b *testing.B) {
	benchmarkRead(b, 4*mebi, NoDelay)
}

func BenchmarkRead4MiBDelay10Msec(b *testing.B) {
	benchmarkRead(b, 4*mebi, 10*time.Millisecond)
}

func BenchmarkRead4MiBDelay50Msec(b *testing.B) {
	benchmarkRead(b, 4*mebi, 50*time.Millisecond)
}

func BenchmarkRead4MiBDelay150Msec(b *testing.B) {
	benchmarkRead(b, 4*mebi, 150*time.Millisecond)
}

func benchmarkWrite(b *testing.B, bufsize int, delay time.Duration) {
	size := 10*mebi + 0x123 // ~10MiB

	cl, cmd := testClient(b, ReadWrite, delay)
	defer cmd.Wait()
	defer cl.Close()

	data := make([]byte, size)
	for i := range data {
		data[i] = uint8(i >> ((i % 4) * 8))
	}

	b.SetBytes(int64(size))

	for b.Loop() {
		func() {
			offset := 0

			f, err := os.CreateTemp("", "sftptest-benchwrite")
			if err != nil {
				b.Fatal(err)
			}
			defer os.Remove(f.Name())
			defer f.Close()

			f2, err := cl.Create(f.Name())
			if err != nil {
				b.Fatal(err)
			}
			defer f2.Close()

			for offset < size {
				buf := data[offset:]
				buf = buf[:min(len(buf), bufsize)]

				n, err := f2.Write(buf)
				offset += n
				if err != nil {
					b.Fatalf("write error at %d: %v", offset, err)
				}

				if n != len(buf) {
					b.Fatalf("wrote too few bytes! written: %d, wanted: %d", n, len(buf))
				}
			}

			fi, err := os.Stat(f.Name())
			if err != nil {
				b.Fatal(err)
			}

			if fi.Size() != int64(size) {
				b.Fatalf("wrong file size: got: %d, want: %d", fi.Size(), size)
			}
		}()
	}
}

func BenchmarkWrite1k(b *testing.B) {
	benchmarkWrite(b, 1*kibi, NoDelay)
}

func BenchmarkWrite16k(b *testing.B) {
	benchmarkWrite(b, 16*kibi, NoDelay)
}

func BenchmarkWrite32k(b *testing.B) {
	benchmarkWrite(b, 32*kibi, NoDelay)
}

func BenchmarkWrite128k(b *testing.B) {
	benchmarkWrite(b, 128*kibi, NoDelay)
}

func BenchmarkWrite512k(b *testing.B) {
	benchmarkWrite(b, 512*kibi, NoDelay)
}

func BenchmarkWrite1MiB(b *testing.B) {
	benchmarkWrite(b, mebi, NoDelay)
}

func BenchmarkWrite4MiB(b *testing.B) {
	benchmarkWrite(b, 4*mebi, NoDelay)
}

func BenchmarkWrite4MiBDelay10Msec(b *testing.B) {
	benchmarkWrite(b, 4*mebi, 10*time.Millisecond)
}

func BenchmarkWrite4MiBDelay50Msec(b *testing.B) {
	benchmarkWrite(b, 4*mebi, 50*time.Millisecond)
}

func BenchmarkWrite4MiBDelay150Msec(b *testing.B) {
	benchmarkWrite(b, 4*mebi, 150*time.Millisecond)
}

func benchmarkReadFrom(b *testing.B, bufsize int, delay time.Duration) {
	size := 10*mebi + 123 // ~10MiB

	// open sftp client
	cl, cmd := testClient(b, ReadWrite, delay)
	defer cmd.Wait()
	defer cl.Close()

	data := make([]byte, size)

	b.SetBytes(int64(size))

	for b.Loop() {
		func() {
			f, err := os.CreateTemp("", "sftptest-benchreadfrom")
			if err != nil {
				b.Fatal(err)
			}
			defer os.Remove(f.Name())
			defer f.Close()

			f2, err := cl.Create(f.Name())
			if err != nil {
				b.Fatal(err)
			}
			defer f2.Close()

			f2.ReadFrom(bytes.NewReader(data))

			fi, err := os.Stat(f.Name())
			if err != nil {
				b.Fatal(err)
			}

			if fi.Size() != int64(size) {
				b.Fatalf("wrong file size: got: %d, want: %d", fi.Size(), size)
			}
		}()
	}
}

func BenchmarkReadFrom1k(b *testing.B) {
	benchmarkReadFrom(b, 1*kibi, NoDelay)
}

func BenchmarkReadFrom16k(b *testing.B) {
	benchmarkReadFrom(b, 16*kibi, NoDelay)
}

func BenchmarkReadFrom32k(b *testing.B) {
	benchmarkReadFrom(b, 32*kibi, NoDelay)
}

func BenchmarkReadFrom128k(b *testing.B) {
	benchmarkReadFrom(b, 128*kibi, NoDelay)
}

func BenchmarkReadFrom512k(b *testing.B) {
	benchmarkReadFrom(b, 512*kibi, NoDelay)
}

func BenchmarkReadFrom1MiB(b *testing.B) {
	benchmarkReadFrom(b, mebi, NoDelay)
}

func BenchmarkReadFrom4MiB(b *testing.B) {
	benchmarkReadFrom(b, 4*mebi, NoDelay)
}

func BenchmarkReadFrom4MiBDelay10Msec(b *testing.B) {
	benchmarkReadFrom(b, 4*mebi, 10*time.Millisecond)
}

func BenchmarkReadFrom4MiBDelay50Msec(b *testing.B) {
	benchmarkReadFrom(b, 4*mebi, 50*time.Millisecond)
}

func BenchmarkReadFrom4MiBDelay150Msec(b *testing.B) {
	benchmarkReadFrom(b, 4*mebi, 150*time.Millisecond)
}

func benchmarkWriteTo(b *testing.B, bufsize int, delay time.Duration) {
	size := 10*mebi + 123 // ~10MiB

	// open sftp client
	cl, cmd := testClient(b, false, delay)
	defer cmd.Wait()
	defer cl.Close()

	f, err := os.CreateTemp("", "sftptest-benchwriteto")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(f.Name())

	data := make([]byte, size)

	if _, err = f.Write(data); err != nil {
		b.Fatal(err)
	}

	if err = f.Close(); err != nil {
		b.Fatal(err)
	}

	buf := bytes.NewBuffer(make([]byte, 0, size))

	b.SetBytes(int64(size))

	for i := 0; i < b.N; i++ {
		buf.Reset()

		f2, err := cl.Open(f.Name())
		if err != nil {
			b.Fatal(err)
		}

		if _, err = f2.WriteTo(buf); err != nil {
			b.Fatal(err)
		}

		if err = f2.Close(); err != nil {
			b.Fatal(err)
		}

		if buf.Len() != size {
			b.Fatalf("wrong buffer size: got: %d, want: %d", buf.Len(), size)
		}
	}
}

func BenchmarkWriteTo1k(b *testing.B) {
	benchmarkWriteTo(b, 1*kibi, NoDelay)
}

func BenchmarkWriteTo16k(b *testing.B) {
	benchmarkWriteTo(b, 16*kibi, NoDelay)
}

func BenchmarkWriteTo32k(b *testing.B) {
	benchmarkWriteTo(b, 32*kibi, NoDelay)
}

func BenchmarkWriteTo128k(b *testing.B) {
	benchmarkWriteTo(b, 128*kibi, NoDelay)
}

func BenchmarkWriteTo512k(b *testing.B) {
	benchmarkWriteTo(b, 512*kibi, NoDelay)
}

func BenchmarkWriteTo1MiB(b *testing.B) {
	benchmarkWriteTo(b, mebi, NoDelay)
}

func BenchmarkWriteTo4MiB(b *testing.B) {
	benchmarkWriteTo(b, 4*mebi, NoDelay)
}

func BenchmarkWriteTo4MiBDelay10Msec(b *testing.B) {
	benchmarkWriteTo(b, 4*mebi, 10*time.Millisecond)
}

func BenchmarkWriteTo4MiBDelay50Msec(b *testing.B) {
	benchmarkWriteTo(b, 4*mebi, 50*time.Millisecond)
}

func BenchmarkWriteTo4MiBDelay150Msec(b *testing.B) {
	benchmarkWriteTo(b, 4*mebi, 150*time.Millisecond)
}

type zeroSource struct{}

func (zeroSource) Read(b []byte) (n int, err error) {
	for i := range b {
		b[i] = 0
	}
	return len(b), nil
}

func (zeroSource) Close() error { return nil }

func zeroFile(t testing.TB, filename string, filesize int64) string {
	src, err := os.CreateTemp("", filename)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	n, err := io.Copy(src, io.LimitReader(zeroSource{}, filesize))
	if err != nil {
		t.Fatal(err)
	}

	if n < filesize {
		t.Fatal("short copy")
	}

	return src.Name()
}

func benchmarkCopyDown(b *testing.B, filesize int64, delay time.Duration) {
	srcFilename := zeroFile(b, "sftptest-benchcopydown-src", filesize)
	defer os.Remove(srcFilename)

	cl, cmd := testClient(b, ReadOnly, delay)
	defer cmd.Wait()
	defer cl.Close()

	b.SetBytes(filesize)

	for b.Loop() {
		func() {
			dst, err := os.CreateTemp("", "sftptest-benchcopydown-dst")
			if err != nil {
				b.Fatal(err)
			}
			defer os.Remove(dst.Name())
			defer dst.Close()

			src, err := cl.Open(srcFilename)
			if err != nil {
				b.Fatal(err)
			}
			defer src.Close()

			n, err := io.Copy(dst, src)
			if err != nil {
				b.Fatal("copy error:", err)
			}

			if n != filesize {
				b.Fatalf("wrong bytes copied: got: %d, want: %d", n, filesize)
			}

			fi, err := dst.Stat()
			if err != nil {
				b.Fatal(err)
			}

			if fi.Size() != filesize {
				b.Fatalf("wrong file size: got: %d, want: %d", fi.Size(), filesize)
			}
		}()
	}
}

func BenchmarkCopyDown10MiBDelay10Msec(b *testing.B) {
	benchmarkCopyDown(b, 10*mebi, 10*time.Millisecond)
}

func BenchmarkCopyDown10MiBDelay50Msec(b *testing.B) {
	benchmarkCopyDown(b, 10*mebi, 50*time.Millisecond)
}

func BenchmarkCopyDown10MiBDelay150Msec(b *testing.B) {
	benchmarkCopyDown(b, 10*mebi, 150*time.Millisecond)
}

func benchmarkCopyUp(b *testing.B, filesize int64, delay time.Duration) {
	srcFilename := zeroFile(b, "sftptest-benchcopyup-src", filesize)
	defer os.Remove(srcFilename)

	sftp, cmd := testClient(b, false, delay)
	defer cmd.Wait()
	defer sftp.Close()

	b.SetBytes(filesize)

	for b.Loop() {
		func() {
			// We need to create the destination filename through the OS first.
			tmpDst, err := os.CreateTemp("", "sftptest-benchcopyup-dst")
			if err != nil {
				b.Fatal(err)
			}
			dstFilename := tmpDst.Name()
			tmpDst.Close()

			defer os.Remove(dstFilename)

			// Now, we can use the filename from above as our destination.
			dst, err := sftp.Create(dstFilename)
			if err != nil {
				b.Fatal(err)
			}
			defer dst.Close()

			src, err := os.Open(srcFilename)
			if err != nil {
				b.Fatal(err)
			}
			defer src.Close()

			n, err := io.Copy(dst, src)
			if err != nil {
				b.Fatal("copy error:", err)
			}

			if n < filesize {
				b.Error("unable to copy all bytes")
			}

			fi, err := os.Stat(dstFilename)
			if err != nil {
				b.Fatal(err)
			}

			if fi.Size() != filesize {
				b.Errorf("wrong file size: got %d, want %d", fi.Size(), filesize)
			}
		}()
	}
}

func BenchmarkCopyUp10MiBDelay10Msec(b *testing.B) {
	benchmarkCopyUp(b, 10*mebi, 10*time.Millisecond)
}

func BenchmarkCopyUp10MiBDelay50Msec(b *testing.B) {
	benchmarkCopyUp(b, 10*mebi, 50*time.Millisecond)
}

func BenchmarkCopyUp10MiBDelay150Msec(b *testing.B) {
	benchmarkCopyUp(b, 10*mebi, 150*time.Millisecond)
}
