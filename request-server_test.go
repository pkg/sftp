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
	_, rs := clientRequestServerPair(t)
	foo := &Request{Filepath: "foo"}
	bar := &Request{Filepath: "bar"}
	rs.nextRequest(foo)
	rs.nextRequest(bar)
	assert.Len(t, rs.openRequests, 2)
	_foo, ok := rs.getRequest("foo")
	assert.Equal(t, foo, _foo)
	assert.True(t, ok)
	_, ok = rs.getRequest("zed")
	assert.False(t, ok)
	rs.closeRequest("foo")
	rs.closeRequest("bar")
	assert.Len(t, rs.openRequests, 0)
}
