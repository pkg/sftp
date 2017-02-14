package sftp

import (
	"fmt"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ = fmt.Print

type csPair struct {
	cli *Client
	svr *RequestServer
}

// these must be closed in order, else client.Close will hang
func (cs csPair) Close() {
	cs.svr.Close()
	cs.cli.Close()
}

func (cs csPair) testHandler() *root {
	return cs.svr.Handlers.FileGet.(*root)
}

func clientRequestServerPair(t *testing.T) *csPair {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	handlers := InMemHandler()
	server := NewRequestServer(struct {
		io.Reader
		io.WriteCloser
	}{sr, sw}, handlers)
	go server.Serve()
	client, err := NewClientPipe(cr, cw)
	if err != nil {
		t.Fatalf("%+v\n", err)
	}
	return &csPair{client, server}
}

func TestRequestCache(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	foo := &Request{Filepath: "foo"}
	bar := &Request{Filepath: "bar"}
	fh := p.svr.nextRequest(foo)
	bh := p.svr.nextRequest(bar)
	assert.Len(t, p.svr.openRequests, 2)
	_foo, ok := p.svr.getRequest(fh)
	assert.Equal(t, foo, _foo)
	assert.True(t, ok)
	_, ok = p.svr.getRequest("zed")
	assert.False(t, ok)
	p.svr.closeRequest(fh)
	p.svr.closeRequest(bh)
	assert.Len(t, p.svr.openRequests, 0)
}

func putTestFile(cli *Client, path, content string) (int, error) {
	w, err := cli.Create(path)
	if err == nil {
		defer w.Close()
		return w.Write([]byte(content))
	}
	return 0, err
}

func TestRequestWrite(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	n, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	assert.Equal(t, 5, n)
	r := p.testHandler()
	f, err := r.fetch("/foo")
	assert.Nil(t, err)
	assert.False(t, f.isdir)
	assert.Equal(t, f.content, []byte("hello"))
}

// needs fail check
func TestRequestFilename(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	r := p.testHandler()
	f, err := r.fetch("/foo")
	assert.Nil(t, err)
	assert.Equal(t, f.Name(), "foo")
}

func TestRequestRead(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	rf, err := p.cli.Open("/foo")
	assert.Nil(t, err)
	defer rf.Close()
	contents := make([]byte, 5)
	n, err := rf.Read(contents)
	if err != nil && err != io.EOF {
		t.Fatalf("err: %v", err)
	}
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(contents[0:5]))
}

func TestRequestReadFail(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	rf, err := p.cli.Open("/foo")
	assert.Nil(t, err)
	contents := make([]byte, 5)
	n, err := rf.Read(contents)
	assert.Equal(t, n, 0)
	assert.IsType(t, &StatusError{}, err)
}

func TestRequestOpen(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	fh, err := p.cli.Open("foo")
	assert.Nil(t, err)
	err = fh.Close()
	assert.Nil(t, err)
}

func TestRequestMkdir(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	err := p.cli.Mkdir("/foo")
	assert.Nil(t, err)
	r := p.testHandler()
	f, err := r.fetch("/foo")
	assert.Nil(t, err)
	assert.True(t, f.isdir)
}

func TestRequestRemove(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	r := p.testHandler()
	_, err = r.fetch("/foo")
	assert.Nil(t, err)
	err = p.cli.Remove("/foo")
	assert.Nil(t, err)
	_, err = r.fetch("/foo")
	assert.Equal(t, err, os.ErrNotExist)
}

func TestRequestRename(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	r := p.testHandler()
	_, err = r.fetch("/foo")
	assert.Nil(t, err)
	err = p.cli.Rename("/foo", "/bar")
	assert.Nil(t, err)
	_, err = r.fetch("/bar")
	assert.Nil(t, err)
	_, err = r.fetch("/foo")
	assert.Equal(t, err, os.ErrNotExist)
}

func TestRequestRenameFail(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	_, err = putTestFile(p.cli, "/bar", "goodbye")
	assert.Nil(t, err)
	err = p.cli.Rename("/foo", "/bar")
	assert.IsType(t, &StatusError{}, err)
}

func TestRequestStat(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	fi, err := p.cli.Stat("/foo")
	assert.Equal(t, fi.Name(), "foo")
	assert.Equal(t, fi.Size(), int64(5))
	assert.Equal(t, fi.Mode(), os.FileMode(0644))
	fstat := fi.Sys().(*FileStat)
	assert.Equal(t, fstat.UID, uint32(65534))
	assert.Equal(t, fstat.GID, uint32(65534))
}

func TestRequestStatFail(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	fi, err := p.cli.Stat("/foo")
	assert.Nil(t, fi)
	assert.True(t, os.IsNotExist(err))
}

func TestRequestSymlink(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	err = p.cli.Symlink("/foo", "/bar")
	assert.Nil(t, err)
	r := p.testHandler()
	fi, err := r.fetch("/bar")
	assert.Nil(t, err)
	assert.True(t, fi.Mode()&os.ModeSymlink == os.ModeSymlink)
}

func TestRequestSymlinkFail(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	err := p.cli.Symlink("/foo", "/bar")
	assert.True(t, os.IsNotExist(err))
}

func TestRequestReadlink(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	err = p.cli.Symlink("/foo", "/bar")
	assert.Nil(t, err)
	rl, err := p.cli.ReadLink("/bar")
	assert.Nil(t, err)
	assert.Equal(t, "foo", rl)
}

func TestRequestReaddir(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	_, err = putTestFile(p.cli, "/bar", "goodbye")
	assert.Nil(t, err)
	di, err := p.cli.ReadDir("/")
	assert.Nil(t, err)
	assert.Len(t, di, 2)
	names := []string{di[0].Name(), di[1].Name()}
	sort.Strings(names)
	assert.Equal(t, []string{"bar", "foo"}, names)
}
