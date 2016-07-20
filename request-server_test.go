package sftp

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

type csPair struct {
	cli *Client
	svr *RequestServer
}

// these must be closed in order, else client.Close will hang
func (cs csPair) Close() {
	cs.svr.Close()
	cs.cli.Close()
}

func clientRequestServerPair(t *testing.T) *csPair {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	handlers := InMemHandler()
	server, err := NewRequestServer(struct {
		io.Reader
		io.WriteCloser
	}{sr, sw}, handlers)
	if err != nil { t.Fatal(err) }
	go server.Serve()
	client, err := NewClientPipe(cr, cw)
	if err != nil { t.Fatalf("%+v\n", err) }
	return &csPair{client, server}
}

func TestRequestCache(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	foo := &Request{Filepath: "foo"}
	bar := &Request{Filepath: "bar"}
	p.svr.nextRequest(foo)
	p.svr.nextRequest(bar)
	assert.Len(t, p.svr.openRequests, 2)
	_foo, ok := p.svr.getRequest("foo")
	assert.Equal(t, foo, _foo)
	assert.True(t, ok)
	_, ok = p.svr.getRequest("zed")
	assert.False(t, ok)
	p.svr.closeRequest("foo")
	p.svr.closeRequest("bar")
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
	if err != nil { t.Fatal(err) }
	assert.Equal(t, 5, n)
}

func TestRequestRead(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()
	_, err := putTestFile(p.cli, "/foo", "hello")
	assert.Nil(t, err)
	rf, err := p.cli.Open("/foo")
	assert.Nil(t, err)
	defer rf.Close()
	// defer rf.Close()
	contents := make([]byte, 5)
	n, err := rf.Read(contents)
	if err != nil && err != io.EOF { t.Fatalf("err: %v", err) }
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(contents[0:5]))
}
