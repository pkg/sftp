package sftp

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

// testFile is an in-memory FileHandler that implements none of the optional FSETSTAT interfaces.
type testFile struct {
	mu   sync.Mutex
	name string
	data []byte
}

func (f *testFile) Close() error   { return nil }
func (f *testFile) Name() string   { return f.name }
func (f *testFile) Handle() string { return f.name }
func (f *testFile) Sync() error    { return nil }

func (f *testFile) Stat() (*sshfx.Attributes, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	attrs := new(sshfx.Attributes)
	attrs.SetSize(uint64(len(f.data)))
	return attrs, nil
}

func (f *testFile) ReadAt(b []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if off >= int64(len(f.data)) {
		return 0, io.EOF
	}

	n := copy(b, f.data[off:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

func (f *testFile) WriteAt(b []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if end := off + int64(len(b)); end > int64(len(f.data)) {
		f.data = append(f.data, make([]byte, end-int64(len(f.data)))...)
	}

	return copy(f.data[off:], b), nil
}

func (f *testFile) contents() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return string(f.data)
}

// truncatingFile only implements TruncateFileHandler out of the optional FSETSTAT interfaces.
type truncatingFile struct {
	*testFile
}

func (f truncatingFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.data = f.data[:size]
	return nil
}

type testHandler struct {
	UnimplementedServerHandler

	file FileHandler
}

func (h *testHandler) Open(_ context.Context, req *sshfx.OpenPacket) (FileHandler, error) {
	if h.file == nil {
		return nil, sshfx.StatusNoSuchFile
	}

	return h.file, nil
}

// serveTestClient starts srv on one end of a pipe, and returns a client connected to the other end.
func serveTestClient(t *testing.T, srv *Server) *Client {
	t.Helper()

	srvConn, clConn := net.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- srv.Serve(srvConn)
	}()

	cl, err := NewClientPipe(t.Context(), clConn, clConn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cl.Close()
		<-done
	})

	return cl
}

func TestServerFSetStatFallback(t *testing.T) {
	f := &testFile{name: "foo", data: []byte("hello world")}

	cl := serveTestClient(t, &Server{
		Handler: &testHandler{file: truncatingFile{f}},
	})

	file, err := cl.OpenFile("foo", OpenFlagReadWrite, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	if err := file.Truncate(5); err != nil {
		t.Fatal("Truncate:", err)
	}

	if got := f.contents(); got != "hello" {
		t.Errorf("after Truncate(5), file contains %q, but wanted %q", got, "hello")
	}

	if err := file.Chmod(0o600); !errors.Is(err, sshfx.StatusOpUnsupported) {
		t.Errorf("Chmod on a file without ChmodFileHandler returned %v, but wanted %v", err, sshfx.StatusOpUnsupported)
	}
}

func TestServerHijack(t *testing.T) {
	srv := &Server{
		Handler: &testHandler{},
	}

	var got string
	err := Hijack(srv, func(_ context.Context, req *sshfx.MkdirPacket) error {
		got = req.Path
		return nil
	})
	if err != nil {
		t.Fatal("Hijack:", err)
	}

	cl := serveTestClient(t, srv)

	if err := cl.Mkdir("foo", 0o755); err != nil {
		t.Fatal("Mkdir:", err)
	}

	if got != "foo" {
		t.Errorf("hijacked mkdir got path %q, but wanted %q", got, "foo")
	}
}

func TestServerHijackWithResponse(t *testing.T) {
	srv := &Server{
		Handler: &testHandler{},
	}

	err := HijackWithResponse(srv, func(_ context.Context, req *sshfx.StatPacket) (*sshfx.AttrsPacket, error) {
		if req.Path != "foo" {
			return nil, sshfx.StatusNoSuchFile
		}

		resp := new(sshfx.AttrsPacket)
		resp.Attrs.SetSize(42)
		return resp, nil
	})
	if err != nil {
		t.Fatal("HijackWithResponse:", err)
	}

	cl := serveTestClient(t, srv)

	fi, err := cl.Stat("foo")
	if err != nil {
		t.Fatal("Stat:", err)
	}

	if fi.Size() != 42 {
		t.Errorf("hijacked stat returned size %d, but wanted 42", fi.Size())
	}

	if _, err := cl.Stat("bar"); !errors.Is(err, sshfx.StatusNoSuchFile) {
		t.Errorf("hijacked stat of a missing file returned %v, but wanted %v", err, sshfx.StatusNoSuchFile)
	}
}

func TestServerGracefulStopBeforeServe(t *testing.T) {
	srv := &Server{
		Handler: &testHandler{},
	}

	if err := srv.GracefulStop(); err != nil {
		t.Error("GracefulStop:", err)
	}
}

// expectWithin fails the test if fn does not return within a second.
func expectWithin(t *testing.T, what string, fn func()) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal(what, "did not return")
	}
}

func TestServerServeTwice(t *testing.T) {
	srv := &Server{
		Handler: &testHandler{},
	}

	cl := serveTestClient(t, srv)

	conn, _ := net.Pipe()
	defer conn.Close()

	if err := srv.Serve(conn); err == nil {
		t.Fatal("expected an error from serving twice")
	}

	// The first Serve is still running, so the server must still answer requests.
	if _, err := cl.RealPath("."); !errors.Is(err, sshfx.StatusOpUnsupported) {
		t.Fatalf("RealPath returned %v, but wanted %v", err, sshfx.StatusOpUnsupported)
	}

	cl.Close()

	expectWithin(t, "GracefulStop", func() {
		srv.GracefulStop()
	})
}

func TestServerGracefulStopWithOpenFile(t *testing.T) {
	srv := &Server{
		Handler: &testHandler{file: &testFile{name: "foo"}},
	}

	cl := serveTestClient(t, srv)

	if _, err := cl.Open("foo"); err != nil {
		t.Fatal(err)
	}

	// Leave the file open, and end the session so that Serve returns.
	cl.Close()

	expectWithin(t, "GracefulStop", func() {
		if err := srv.GracefulStop(); err != nil {
			t.Error("GracefulStop:", err)
		}
	})
}
