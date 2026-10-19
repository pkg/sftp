//go:build integration && !windows

package sftp

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"testing"
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
