package sftp

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func clientRequestServerPair(t *testing.T) *csPair {
	ready := make(chan string)
	var server *RequestServer
	go func() {
		l, err := net.Listen("tcp", "localhost:0")
		if err != nil {
			// neither assert nor t.Fatal reliably exit before Accept errors
			panic(err)
		}
		ready <- l.Addr().String()
		fd, err := l.Accept()
		assert.Nil(t, err)
		handlers := InMemHandler()
		server = NewRequestServer(fd, handlers)
		server.Serve()
	}()
	addr := <-ready
	c, err := net.Dial("tcp", addr)
	assert.Nil(t, err)
	client, err := NewClientPipe(c, c)
	if err != nil {
		t.Fatalf("%+v\n", err)
	}
	return &csPair{client, server, nil}
}
