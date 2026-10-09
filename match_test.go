package sftp

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobLiteralPaths(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	server := NewRequestServer(serverConn, InMemHandler())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer server.Close()
		server.Serve()
	}()
	t.Cleanup(func() {
		clientConn.Close()
		<-done
	})
	client, err := NewClientPipe(clientConn, clientConn)
	require.NoError(t, err)
	defer client.Close()

	require.NoError(t, client.Mkdir("/home"))
	file, err := client.Create("/home/file")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	for _, pattern := range []string{
		"/home/", "home/", "./home/", "/home//", "/home/./", "/home/../home/",
		"/", ".", "/home", "home/file", "./home/file", "/home/file",
	} {
		t.Run(pattern, func(t *testing.T) {
			matches, err := client.Glob(pattern)
			require.NoError(t, err)
			assert.Equal(t, []string{pattern}, matches)
		})
	}

	matches, err := client.Glob("/missing")
	require.NoError(t, err)
	assert.Nil(t, matches)
	matches, err = client.Glob("/home/*")
	require.NoError(t, err)
	assert.Equal(t, []string{"/home/file"}, matches)
	_, err = client.Glob("[")
	assert.ErrorIs(t, err, ErrBadPattern)
}
