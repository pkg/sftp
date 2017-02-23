//+build !windows

package sftp

import (
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const sock = "/tmp/rstest.sock"

func clientRequestServerPair(t *testing.T) *csPair {
	ready := make(chan bool)
	os.Remove(sock) // either this or signal handling
	var server *RequestServer
	go func() {
		l, err := net.Listen("unix", sock)
		if err != nil {
			// neither assert nor t.Fatal reliably exit before Accept errors
			panic(err)
		}
		ready <- true
		fd, err := l.Accept()
		assert.Nil(t, err)
		handlers := InMemHandler()
		server = NewRequestServer(fd, handlers)
		server.Serve()
	}()
	<-ready
	defer os.Remove(sock)
	c, err := net.Dial("unix", sock)
	assert.Nil(t, err)
	client, err := NewClientPipe(c, c)
	if err != nil {
		t.Fatalf("%+v\n", err)
	}
	return &csPair{client, server, func() { os.Remove(sock) }}
}
