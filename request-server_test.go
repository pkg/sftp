package sftp

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ = fmt.Print

type csPair struct {
	cli       *Client
	svr       *RequestServer
	svrResult chan error
}

// these must be closed in order, else client.Close will hang
func (cs csPair) Close() {
	cs.svr.Close()
	cs.cli.Close()
	os.Remove(sock)
}

func (cs csPair) testHandler() *root {
	return cs.svr.Handlers.FileGet.(*root)
}

const sock = "/tmp/rstest.sock"

func clientRequestServerPair(t *testing.T) *csPair {
	skipIfWindows(t)
	skipIfPlan9(t)
	ready := make(chan bool)
	os.Remove(sock) // either this or signal handling
	pair := &csPair{
		svrResult: make(chan error, 1),
	}
	var server *RequestServer
	go func() {
		l, err := net.Listen("unix", sock)
		if err != nil {
			// neither assert nor t.Fatal reliably exit before Accept errors
			panic(err)
		}
		ready <- true
		fd, err := l.Accept()
		require.NoError(t, err)
		handlers := InMemHandler()
		var options []RequestServerOption
		if *testAllocator {
			options = append(options, WithRSAllocator())
		}
		server = NewRequestServer(fd, handlers, options...)
		err = server.Serve()
		pair.svrResult <- err
	}()
	<-ready
	defer os.Remove(sock)
	c, err := net.Dial("unix", sock)
	require.NoError(t, err)
	client, err := NewClientPipe(c, c)
	if err != nil {
		t.Fatalf("%+v\n", err)
	}
	pair.svr = server
	pair.cli = client
	return pair
}

func checkRequestServerAllocator(t *testing.T, p *csPair) {
	if p.svr.pktMgr.alloc == nil {
		return
	}
	checkAllocatorBeforeServerClose(t, p.svr.pktMgr.alloc)
	p.Close()
	checkAllocatorAfterServerClose(t, p.svr.pktMgr.alloc)
}

// after adding logging, maybe check log to make sure packet handling
// was split over more than one worker
func TestRequestSplitWrite(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	w, err := p.cli.Create("/foo")
	require.NoError(t, err)
	p.cli.maxPacket = 3 // force it to send in small chunks
	contents := "one two three four five six seven eight nine ten"
	w.Write([]byte(contents))
	w.Close()
	r := p.testHandler()
	f, err := r.fetch("/foo")
	require.NoError(t, err)
	assert.Equal(t, contents, string(f.content))
	checkRequestServerAllocator(t, p)
}

func TestRequestCache(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	foo := NewRequest("", "foo")
	foo.ctx, foo.cancelCtx = context.WithCancel(context.Background())
	bar := NewRequest("", "bar")
	fh := p.svr.nextRequest(foo)
	bh := p.svr.nextRequest(bar)
	assert.Len(t, p.svr.openRequests, 2)
	_foo, ok := p.svr.getRequest(fh)
	assert.Equal(t, foo.Method, _foo.Method)
	assert.Equal(t, foo.Filepath, _foo.Filepath)
	assert.Equal(t, foo.Target, _foo.Target)
	assert.Equal(t, foo.Flags, _foo.Flags)
	assert.Equal(t, foo.Attrs, _foo.Attrs)
	assert.Equal(t, foo.state, _foo.state)
	assert.NotNil(t, _foo.ctx)
	assert.Equal(t, _foo.Context().Err(), nil, "context is still valid")
	assert.True(t, ok)
	_, ok = p.svr.getRequest("zed")
	assert.False(t, ok)
	p.svr.closeRequest(fh)
	assert.Equal(t, _foo.Context().Err(), context.Canceled, "context is now canceled")
	p.svr.closeRequest(bh)
	assert.Len(t, p.svr.openRequests, 0)
	checkRequestServerAllocator(t, p)
}

func TestRequestCacheState(t *testing.T) {
	// test operation that uses open/close
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	assert.Len(t, p.svr.openRequests, 0)
	// test operation that doesn't open/close
	err = p.cli.Remove("/foo")
	assert.NoError(t, err)
	assert.Len(t, p.svr.openRequests, 0)
	checkRequestServerAllocator(t, p)
}

func putTestFile(cli *Client, path, content string) (int, error) {
	w, err := cli.Create(path)
	if err == nil {
		defer w.Close()
		return w.Write([]byte(content))
	}
	return 0, err
}

func getTestFile(cli *Client, path string) ([]byte, error) {
	r, err := cli.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return ioutil.ReadAll(r)
}

func TestRequestWrite(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	n, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	r := p.testHandler()
	f, err := r.fetch("/foo")
	require.NoError(t, err)
	assert.False(t, f.isdir)
	assert.Equal(t, f.content, []byte("hello"))
	checkRequestServerAllocator(t, p)
}

func TestRequestWriteEmpty(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	n, err := putTestFile(p.cli, "/foo", "")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	r := p.testHandler()
	f, err := r.fetch("/foo")
	require.NoError(t, err)
	assert.False(t, f.isdir)
	assert.Len(t, f.content, 0)
	// lets test with an error
	r.returnErr(os.ErrInvalid)
	n, err = putTestFile(p.cli, "/bar", "")
	require.Error(t, err)
	r.returnErr(nil)
	assert.Equal(t, 0, n)
	checkRequestServerAllocator(t, p)
}

func TestRequestFilename(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	r := p.testHandler()
	f, err := r.fetch("/foo")
	require.NoError(t, err)
	assert.Equal(t, f.Name(), "foo")
	_, err = r.fetch("/bar")
	assert.Error(t, err)
	checkRequestServerAllocator(t, p)
}

func TestRequestJustRead(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	rf, err := p.cli.Open("/foo")
	require.NoError(t, err)
	defer rf.Close()
	contents := make([]byte, 5)
	n, err := rf.Read(contents)
	if err != nil && err != io.EOF {
		t.Fatalf("err: %v", err)
	}
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(contents[0:5]))
	checkRequestServerAllocator(t, p)
}

func TestRequestOpenFail(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	rf, err := p.cli.Open("/foo")
	assert.Exactly(t, os.ErrNotExist, err)
	assert.Nil(t, rf)
	// if we return an error the sftp client will not close the handle
	// ensure that we close it ourself
	assert.Len(t, p.svr.openRequests, 0)
	checkRequestServerAllocator(t, p)
}

func TestRequestCreate(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	fh, err := p.cli.Create("foo")
	require.NoError(t, err)
	err = fh.Close()
	assert.NoError(t, err)
	checkRequestServerAllocator(t, p)
}

func TestRequestReadAndWrite(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	file, err := p.cli.OpenFile("/foo", os.O_RDWR|os.O_CREATE)
	require.NoError(t, err)
	defer file.Close()

	n, err := file.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)

	buf := make([]byte, 4)
	n, err = file.ReadAt(buf, 1)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
	assert.Equal(t, []byte{'e', 'l', 'l', 'o'}, buf)

	checkRequestServerAllocator(t, p)
}

func TestOpenFileExclusive(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	// first open should work
	file, err := p.cli.OpenFile("/foo", os.O_RDWR|os.O_CREATE|os.O_EXCL)
	require.NoError(t, err)
	file.Close()

	// second open should return error
	_, err = p.cli.OpenFile("/foo", os.O_RDWR|os.O_CREATE|os.O_EXCL)
	assert.Error(t, err)

	checkRequestServerAllocator(t, p)
}

func TestOpenFileExclusiveNoSymlinkFollowing(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	// make a directory
	err := p.cli.Mkdir("/foo")
	require.NoError(t, err)

	// make a symlink to that directory
	err = p.cli.Symlink("/foo", "/foo2")
	require.NoError(t, err)

	// with O_EXCL, we can follow directory symlinks
	file, err := p.cli.OpenFile("/foo2/bar", os.O_RDWR|os.O_CREATE|os.O_EXCL)
	require.NoError(t, err)
	err = file.Close()
	require.NoError(t, err)

	// we should have created the file above; and this create should fail.
	_, err = p.cli.OpenFile("/foo/bar", os.O_RDWR|os.O_CREATE|os.O_EXCL)
	require.Error(t, err)

	// create a dangling symlink
	err = p.cli.Symlink("/notexist", "/bar")
	require.NoError(t, err)

	// opening a dangling symlink with O_CREATE and O_EXCL should fail, regardless of target not existing.
	_, err = p.cli.OpenFile("/bar", os.O_RDWR|os.O_CREATE|os.O_EXCL)
	require.Error(t, err)

	checkRequestServerAllocator(t, p)
}

func TestRequestMkdir(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	err := p.cli.Mkdir("/foo")
	require.NoError(t, err)
	r := p.testHandler()
	f, err := r.fetch("/foo")
	require.NoError(t, err)
	assert.True(t, f.IsDir())
	checkRequestServerAllocator(t, p)
}

func TestRequestRemove(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	r := p.testHandler()
	_, err = r.fetch("/foo")
	assert.NoError(t, err)
	err = p.cli.Remove("/foo")
	assert.NoError(t, err)
	_, err = r.fetch("/foo")
	assert.Equal(t, err, os.ErrNotExist)
	checkRequestServerAllocator(t, p)
}

func TestRequestRename(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	content, err := getTestFile(p.cli, "/foo")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), content)

	err = p.cli.Rename("/foo", "/bar")
	require.NoError(t, err)

	// file contents are now at /bar
	content, err = getTestFile(p.cli, "/bar")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), content)

	// /foo no longer exists
	_, err = getTestFile(p.cli, "/foo")
	require.Error(t, err)

	_, err = putTestFile(p.cli, "/baz", "goodbye")
	require.NoError(t, err)
	content, err = getTestFile(p.cli, "/baz")
	require.NoError(t, err)
	require.Equal(t, []byte("goodbye"), content)

	// SFTP-v2: SSH_FXP_RENAME may not overwrite existing files.
	err = p.cli.Rename("/bar", "/baz")
	require.Error(t, err)

	// /bar and /baz are unchanged
	content, err = getTestFile(p.cli, "/bar")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), content)
	content, err = getTestFile(p.cli, "/baz")
	require.NoError(t, err)
	require.Equal(t, []byte("goodbye"), content)

	// posix-rename@openssh.com extension allows overwriting existing files.
	err = p.cli.PosixRename("/bar", "/baz")
	require.NoError(t, err)

	// /baz now has the contents of /bar
	content, err = getTestFile(p.cli, "/baz")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), content)

	// /bar no longer exists
	_, err = getTestFile(p.cli, "/bar")
	require.Error(t, err)

	checkRequestServerAllocator(t, p)
}

func TestRequestRenameFail(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	_, err = putTestFile(p.cli, "/bar", "goodbye")
	require.NoError(t, err)
	err = p.cli.Rename("/foo", "/bar")
	assert.IsType(t, &StatusError{}, err)
	checkRequestServerAllocator(t, p)
}

func TestRequestStat(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	fi, err := p.cli.Stat("/foo")
	require.NoError(t, err)
	assert.Equal(t, fi.Name(), "foo")
	assert.Equal(t, fi.Size(), int64(5))
	assert.Equal(t, fi.Mode(), os.FileMode(0644))
	assert.NoError(t, testOsSys(fi.Sys()))
	checkRequestServerAllocator(t, p)
}

// NOTE: Setstat is a noop in the request server tests, but we want to test
// that is does nothing without crapping out.
func TestRequestSetstat(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	mode := os.FileMode(0644)
	err = p.cli.Chmod("/foo", mode)
	require.NoError(t, err)
	fi, err := p.cli.Stat("/foo")
	require.NoError(t, err)
	assert.Equal(t, fi.Name(), "foo")
	assert.Equal(t, fi.Size(), int64(5))
	assert.Equal(t, fi.Mode(), os.FileMode(0644))
	assert.NoError(t, testOsSys(fi.Sys()))
	checkRequestServerAllocator(t, p)
}

func TestRequestFstat(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	fp, err := p.cli.Open("/foo")
	require.NoError(t, err)
	fi, err := fp.Stat()
	require.NoError(t, err)
	assert.Equal(t, fi.Name(), "foo")
	assert.Equal(t, fi.Size(), int64(5))
	assert.Equal(t, fi.Mode(), os.FileMode(0644))
	assert.NoError(t, testOsSys(fi.Sys()))
	checkRequestServerAllocator(t, p)
}

func TestRequestFsetstat(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	fp, err := p.cli.OpenFile("/foo", os.O_WRONLY)
	require.NoError(t, err)
	err = fp.Truncate(2)
	fi, err := fp.Stat()
	require.NoError(t, err)
	assert.Equal(t, fi.Name(), "foo")
	assert.Equal(t, fi.Size(), int64(2))
	err = fp.Truncate(5)
	require.NoError(t, err)
	fi, err = fp.Stat()
	require.NoError(t, err)
	assert.Equal(t, fi.Name(), "foo")
	assert.Equal(t, fi.Size(), int64(5))
	err = fp.Close()
	assert.NoError(t, err)
	rf, err := p.cli.Open("/foo")
	assert.NoError(t, err)
	defer rf.Close()
	contents := make([]byte, 20)
	n, err := rf.Read(contents)
	assert.EqualError(t, err, io.EOF.Error())
	assert.Equal(t, 5, n)
	assert.Equal(t, []byte{'h', 'e', 0, 0, 0}, contents[0:n])
	checkRequestServerAllocator(t, p)
}

func TestRequestStatFail(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	fi, err := p.cli.Stat("/foo")
	assert.Nil(t, fi)
	assert.True(t, os.IsNotExist(err))
	checkRequestServerAllocator(t, p)
}

func TestRequestLstat(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	err = p.cli.Symlink("/foo", "/bar")
	require.NoError(t, err)
	fi, err := p.cli.Lstat("/bar")
	require.NoError(t, err)
	assert.True(t, fi.Mode()&os.ModeSymlink == os.ModeSymlink)
	checkRequestServerAllocator(t, p)
}

func TestRequestLink(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)

	err = p.cli.Link("/foo", "/bar")
	require.NoError(t, err)

	content, err := getTestFile(p.cli, "/bar")
	assert.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)

	checkRequestServerAllocator(t, p)
}

func TestRequestLinkFail(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	err := p.cli.Link("/foo", "/bar")
	t.Log(err)
	assert.True(t, os.IsNotExist(err))
	checkRequestServerAllocator(t, p)
}

func TestRequestSymlink(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)

	err = p.cli.Symlink("/foo", "/bar")
	require.NoError(t, err)
	err = p.cli.Symlink("/bar", "/baz")
	require.NoError(t, err)

	r := p.testHandler()

	fi, err := r.lfetch("/bar")
	require.NoError(t, err)
	assert.True(t, fi.Mode()&os.ModeSymlink == os.ModeSymlink)

	fi, err = r.lfetch("/baz")
	require.NoError(t, err)
	assert.True(t, fi.Mode()&os.ModeSymlink == os.ModeSymlink)

	content, err := getTestFile(p.cli, "/baz")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)

	checkRequestServerAllocator(t, p)
}

func TestRequestSymlinkLoop(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	err := p.cli.Symlink("/foo", "/bar")
	require.NoError(t, err)
	err = p.cli.Symlink("/bar", "/baz")
	require.NoError(t, err)
	err = p.cli.Symlink("/baz", "/foo")
	require.NoError(t, err)

	// test should fail if we reach this point
	timer := time.NewTimer(1 * time.Second)
	defer timer.Stop()

	var content []byte

	done := make(chan struct{})
	go func() {
		defer close(done)

		content, err = getTestFile(p.cli, "/bar")
	}()

	select {
	case <-timer.C:
		t.Fatal("symlink loop following timed out")
		return // just to let the compiler be absolutely sure

	case <-done:
	}

	assert.Error(t, err)
	assert.Len(t, content, 0)

	checkRequestServerAllocator(t, p)
}

func TestRequestSymlinkDanglingFiles(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	// dangling links are ok. We will use "/foo" later.
	err := p.cli.Symlink("/foo", "/bar")
	require.NoError(t, err)

	// creating a symlink in a non-existant directory should fail.
	err = p.cli.Symlink("/dangle", "/foo/bar")
	require.Error(t, err)

	// creating a symlink under a dangling symlink should fail.
	err = p.cli.Symlink("/dangle", "/bar/bar")
	require.Error(t, err)

	// opening a dangling link without O_CREATE should fail with os.IsNotExist == true
	_, err = p.cli.OpenFile("/bar", os.O_RDONLY)
	require.True(t, os.IsNotExist(err))

	// overwriting a symlink is not allowed.
	err = p.cli.Symlink("/dangle", "/bar")
	require.Error(t, err)

	// double symlink
	err = p.cli.Symlink("/bar", "/baz")
	require.NoError(t, err)

	// opening a dangling link with O_CREATE should work.
	_, err = putTestFile(p.cli, "/baz", "hello")
	require.NoError(t, err)

	// dangling link creation should create the target file itself.
	content, err := getTestFile(p.cli, "/foo")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)

	// creating a symlink under a non-directory file should fail.
	err = p.cli.Symlink("/dangle", "/foo/bar")
	assert.Error(t, err)

	checkRequestServerAllocator(t, p)
}

func TestRequestSymlinkDanglingDirectories(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	// dangling links are ok. We will use "/foo" later.
	err := p.cli.Symlink("/foo", "/bar")
	require.NoError(t, err)

	// reading from a dangling symlink should fail.
	_, err = p.cli.ReadDir("/bar")
	require.True(t, os.IsNotExist(err))

	// making a directory on a dangling symlink SHOULD NOT work.
	err = p.cli.Mkdir("/bar")
	require.Error(t, err)

	// ok, now make directory, so we can test make files through the symlink.
	err = p.cli.Mkdir("/foo")
	require.NoError(t, err)

	// should be able to make a file in that symlinked directory.
	_, err = putTestFile(p.cli, "/bar/baz", "hello")
	require.NoError(t, err)

	// dangling directory creation should create the target directory itself.
	content, err := getTestFile(p.cli, "/foo/baz")
	assert.NoError(t, err)
	assert.Equal(t, []byte("hello"), content)

	checkRequestServerAllocator(t, p)
}

func TestRequestReadlink(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	require.NoError(t, err)
	err = p.cli.Symlink("/foo", "/bar")
	require.NoError(t, err)
	rl, err := p.cli.ReadLink("/bar")
	assert.NoError(t, err)
	assert.Equal(t, "foo", rl)
	checkRequestServerAllocator(t, p)
}

func TestRequestReaddir(t *testing.T) {
	p := clientRequestServerPair(t)
	MaxFilelist = 22 // make not divisible by our test amount (100)
	defer p.Close()
	for i := 0; i < 100; i++ {
		fname := fmt.Sprintf("/foo_%02d", i)
		_, err := putTestFile(p.cli, fname, fname)
		if err != nil {
			t.Fatal("expected no error, got:", err)
		}
	}
	_, err := p.cli.ReadDir("/foo_01")
	assert.Equal(t, &StatusError{Code: sshFxFailure,
		msg: " /foo_01: not a directory"}, err)
	_, err = p.cli.ReadDir("/does_not_exist")
	assert.Equal(t, os.ErrNotExist, err)
	di, err := p.cli.ReadDir("/")
	require.NoError(t, err)
	require.Len(t, di, 100)
	names := []string{di[18].Name(), di[81].Name()}
	assert.Equal(t, []string{"foo_18", "foo_81"}, names)
	assert.Len(t, p.svr.openRequests, 0)
	checkRequestServerAllocator(t, p)
}

func TestCleanDisconnect(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	err := p.cli.conn.Close()
	require.NoError(t, err)
	// server must return io.EOF after a clean client close
	// with no pending open requests
	err = <-p.svrResult
	require.EqualError(t, err, io.EOF.Error())
	checkRequestServerAllocator(t, p)
}

func TestUncleanDisconnect(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	foo := NewRequest("", "foo")
	p.svr.nextRequest(foo)
	err := p.cli.conn.Close()
	require.NoError(t, err)
	// the foo request above is still open after the client disconnects
	// so the server will convert io.EOF to io.ErrUnexpectedEOF
	err = <-p.svrResult
	require.EqualError(t, err, io.ErrUnexpectedEOF.Error())
	checkRequestServerAllocator(t, p)
}

func TestCleanPath(t *testing.T) {
	assert.Equal(t, "/", cleanPath("/"))
	assert.Equal(t, "/", cleanPath("."))
	assert.Equal(t, "/", cleanPath("/."))
	assert.Equal(t, "/", cleanPath("/a/.."))
	assert.Equal(t, "/a/c", cleanPath("/a/b/../c"))
	assert.Equal(t, "/a/c", cleanPath("/a/b/../c/"))
	assert.Equal(t, "/a", cleanPath("/a/b/.."))
	assert.Equal(t, "/a/b/c", cleanPath("/a/b/c"))
	assert.Equal(t, "/", cleanPath("//"))
	assert.Equal(t, "/a", cleanPath("/a/"))
	assert.Equal(t, "/a", cleanPath("a/"))
	assert.Equal(t, "/a/b/c", cleanPath("/a//b//c/"))

	// filepath.ToSlash does not touch \ as char on unix systems
	// so os.PathSeparator is used for windows compatible tests
	bslash := string(os.PathSeparator)
	assert.Equal(t, "/", cleanPath(bslash))
	assert.Equal(t, "/", cleanPath(bslash+bslash))
	assert.Equal(t, "/a", cleanPath(bslash+"a"+bslash))
	assert.Equal(t, "/a", cleanPath("a"+bslash))
	assert.Equal(t, "/a/b/c",
		cleanPath(bslash+"a"+bslash+bslash+"b"+bslash+bslash+"c"+bslash))
	assert.Equal(t, "/C:/a", cleanPath("C:"+bslash+"a"))
}
