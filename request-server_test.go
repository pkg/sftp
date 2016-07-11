package sftp

import (
	"github.com/stretchr/testify/assert"
	"io"
	"testing"
)

func clientRequestServerPair(t *testing.T) (*Client, *RequestServer) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	server, err := NewRequestServer(struct {
		io.Reader
		io.WriteCloser
	}{sr, sw})
	if err != nil { t.Fatal(err) }
	go server.Serve()
	client, err := NewClientPipe(cr, cw)
	if err != nil { t.Fatalf("%+v\n", err) }
	return client, server
}

func TestPsRequestCache(t *testing.T) {
	_, ps := clientRequestServerPair(t)
	foo := &Request{Filepath: "foo"}
	bar := &Request{Filepath: "bar"}
	ps.nextRequest(foo)
	ps.nextRequest(bar)
	assert.Len(t, ps.openRequests, 2)
	_foo, ok := ps.getRequest("foo")
	assert.Equal(t, foo, _foo)
	assert.True(t, ok)
	_, ok = ps.getRequest("zed")
	assert.False(t, ok)
	ps.closeRequest("foo")
	ps.closeRequest("bar")
	assert.Len(t, ps.openRequests, 0)
}
