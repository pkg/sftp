package sftp

// sftp integration tests
// enable with -integration

import (
	"crypto/sha1"
	"flag"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const (
	READONLY                = true
	READWRITE               = false
	NO_DELAY  time.Duration = 0

	debuglevel = "ERROR" // set to "DEBUG" for debugging
)

var testServerImpl = flag.Bool("testserver", false, "perform integration tests against sftp package server instance")
var testIntegration = flag.Bool("integration", false, "perform integration tests against sftp server process")
var testSftp = flag.String("sftp", sftpServer, "location of the sftp server binary")

type delayedWrite struct {
	t time.Time
	b []byte
}

// delayedWriter wraps a writer and artificially delays the write. This is
// meant to mimic connections with various latencies. Error's returned from the
// underlying writer will panic so this should only be used over reliable
// connections.
type delayedWriter struct {
	w      io.WriteCloser
	ch     chan delayedWrite
	closed chan struct{}
}

func newDelayedWriter(w io.WriteCloser, delay time.Duration) io.WriteCloser {
	ch := make(chan delayedWrite, 128)
	closed := make(chan struct{})
	go func() {
		for writeMsg := range ch {
			time.Sleep(writeMsg.t.Add(delay).Sub(time.Now()))
			n, err := w.Write(writeMsg.b)
			if err != nil {
				panic("write error")
			}
			if n < len(writeMsg.b) {
				panic("showrt write")
			}
		}
		w.Close()
		close(closed)
	}()
	return delayedWriter{w: w, ch: ch, closed: closed}
}

func (w delayedWriter) Write(b []byte) (int, error) {
	bcopy := make([]byte, len(b))
	copy(bcopy, b)
	w.ch <- delayedWrite{t: time.Now(), b: bcopy}
	return len(b), nil
}

func (w delayedWriter) Close() error {
	close(w.ch)
	<-w.closed
	return nil
}

// netPipe provides a pair of io.ReadWriteClosers connected to each other.
// The functions is identical to os.Pipe with the exception that netPipe
// provides the Read/Close guarentees that os.File derrived pipes do not.
func netPipe(t testing.TB) (io.ReadWriteCloser, io.ReadWriteCloser) {
	type result struct {
		net.Conn
		error
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan result, 1)
	go func() {
		conn, err := l.Accept()
		ch <- result{conn, err}
		err = l.Close()
		if err != nil {
			t.Error(err)
		}
	}()
	c1, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		l.Close() // might cause another in the listening goroutine, but too bad
		t.Fatal(err)
	}
	r := <-ch
	if r.error != nil {
		t.Fatal(err)
	}
	return c1, r.Conn
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
	if _, err := r.Seek(s.offset, os.SEEK_SET); err != nil {
		t.Fatalf("error while seeking with %+v: %v", s, err)
	}
}

func (s seek) current(t *testing.T, r io.ReadSeeker) {
	const mid = seekBytes / 2

	skip := s.offset / 2
	if s.offset > mid {
		skip = -skip
	}

	if _, err := r.Seek(mid, os.SEEK_SET); err != nil {
		t.Fatalf("error seeking to midpoint with %+v: %v", s, err)
	}
	if _, err := r.Seek(skip, os.SEEK_CUR); err != nil {
		t.Fatalf("error seeking from %d with %+v: %v", mid, s, err)
	}
}

func (s seek) end(t *testing.T, r io.ReadSeeker) {
	if _, err := r.Seek(-s.offset, os.SEEK_END); err != nil {
		t.Fatalf("error seeking from end with %+v: %v", s, err)
	}
}

func sameFile(want, got os.FileInfo) bool {
	return want.Name() == got.Name() &&
		want.Size() == got.Size()
}

var clientReadTests = []struct {
	n int64
}{
	{0},
	{1},
	{1000},
	{1024},
	{1025},
	{2048},
	{4096},
	{1 << 12},
	{1 << 13},
	{1 << 14},
	{1 << 15},
	{1 << 16},
	{1 << 17},
	{1 << 18},
	{1 << 19},
	{1 << 20},
}

// readHash reads r until EOF returning the number of bytes read
// and the hash of the contents.
func readHash(t *testing.T, r io.Reader) (string, int64) {
	h := sha1.New()
	tr := io.TeeReader(r, h)
	read, err := io.Copy(ioutil.Discard, tr)
	if err != nil {
		t.Fatal(err)
	}
	return string(h.Sum(nil)), read
}

// writeN writes n bytes of random data to w and returns the
// hash of that data.
func writeN(t *testing.T, w io.Writer, n int64) string {
	rand, err := os.Open("/dev/urandom")
	if err != nil {
		t.Fatal(err)
	}
	defer rand.Close()

	h := sha1.New()

	mw := io.MultiWriter(w, h)

	written, err := io.CopyN(mw, rand, n)
	if err != nil {
		t.Fatal(err)
	}
	if written != n {
		t.Fatalf("CopyN(%v): wrote: %v", n, written)
	}
	return string(h.Sum(nil))
}

var clientWriteTests = []struct {
	n     int
	total int64 // cumulative file size
}{
	{0, 0},
	{1, 1},
	{0, 1},
	{999, 1000},
	{24, 1024},
	{1023, 2047},
	{2048, 4095},
	{1 << 12, 8191},
	{1 << 13, 16383},
	{1 << 14, 32767},
	{1 << 15, 65535},
	{1 << 16, 131071},
	{1 << 17, 262143},
	{1 << 18, 524287},
	{1 << 19, 1048575},
	{1 << 20, 2097151},
	{1 << 21, 4194303},
}

// taken from github.com/kr/fs/walk_test.go

type Node struct {
	name    string
	entries []*Node // nil if the entry is a file
	mark    int
}

var tree = &Node{
	"testdata",
	[]*Node{
		{"a", nil, 0},
		{"b", []*Node{}, 0},
		{"c", nil, 0},
		{
			"d",
			[]*Node{
				{"x", nil, 0},
				{"y", []*Node{}, 0},
				{
					"z",
					[]*Node{
						{"u", nil, 0},
						{"v", nil, 0},
					},
					0,
				},
			},
			0,
		},
	},
	0,
}

func walkTree(n *Node, path string, f func(path string, n *Node)) {
	f(path, n)
	for _, e := range n.entries {
		walkTree(e, filepath.Join(path, e.name), f)
	}
}

func makeTree(t *testing.T) {
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.entries == nil {
			fd, err := os.Create(path)
			if err != nil {
				t.Errorf("makeTree: %v", err)
				return
			}
			fd.Close()
		} else {
			os.Mkdir(path, 0770)
		}
	})
}

func markTree(n *Node) { walkTree(n, "", func(path string, n *Node) { n.mark++ }) }

func checkMarks(t *testing.T, report bool) {
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.mark != 1 && report {
			t.Errorf("node %s mark = %d; expected 1", path, n.mark)
		}
		n.mark = 0
	})
}

// Assumes that each node name is unique. Good enough for a test.
// If clear is true, any incoming error is cleared before return. The errors
// are always accumulated, though.
func mark(path string, info os.FileInfo, err error, errors *[]error, clear bool) error {
	if err != nil {
		*errors = append(*errors, err)
		if clear {
			return nil
		}
		return err
	}
	name := info.Name()
	walkTree(tree, tree.name, func(path string, n *Node) {
		if n.name == name {
			n.mark++
		}
	})
	return nil
}
