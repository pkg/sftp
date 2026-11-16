//go:build linux
// +build linux

package sftp

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func chownClientServerPair(t *testing.T) *Client {
	t.Helper()

	uid, gid := os.Getuid(), os.Getgid()
	if uid <= 0 || gid <= 0 {
		t.Skip("WithServerChown needs a non-root uid and gid")
	}

	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	server, err := NewServer(struct {
		io.Reader
		io.WriteCloser
	}{sr, sw}, WithServerChown(uid, gid))
	require.NoError(t, err)
	go server.Serve()

	client, err := NewClientPipe(cr, cw)
	require.NoError(t, err)
	t.Cleanup(func() {
		// The server has to go first: the client waits for its side of the pipe to end.
		server.Close()
		client.Close()
	})
	return client
}

func ctimeOf(t *testing.T, name string) syscall.Timespec {
	t.Helper()

	var st syscall.Stat_t
	require.NoError(t, syscall.Lstat(name, &st))
	return st.Ctim
}

// existingFile creates a file and waits long enough,
// that any later change of its metadata is visible in its ctime.
func existingFile(t *testing.T) string {
	t.Helper()

	name := filepath.Join(t.TempDir(), "existing.txt")
	require.NoError(t, os.WriteFile(name, []byte("hello"), 0o640))
	time.Sleep(50 * time.Millisecond)
	return name
}

func TestServerChownDoesNotTouchExistingFile(t *testing.T) {
	tests := []struct {
		name  string
		flags int
	}{
		{"read", os.O_RDONLY},
		{"write", os.O_WRONLY},
		{"write create", os.O_WRONLY | os.O_CREATE},
		{"read write create", os.O_RDWR | os.O_CREATE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := chownClientServerPair(t)
			name := existingFile(t)
			before := ctimeOf(t, name)

			f, err := client.OpenFile(name, tt.flags)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			require.Equal(t, before, ctimeOf(t, name), "open must not change the ctime of an existing file")
		})
	}
}

func TestServerChownCreatesFile(t *testing.T) {
	client := chownClientServerPair(t)
	name := filepath.Join(t.TempDir(), "new.txt")

	f, err := client.OpenFile(name, os.O_WRONLY|os.O_CREATE)
	require.NoError(t, err)
	_, err = f.Write([]byte("hello"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	var st syscall.Stat_t
	require.NoError(t, syscall.Lstat(name, &st))
	require.Equal(t, uint32(os.Getuid()), st.Uid)
	require.Equal(t, uint32(os.Getgid()), st.Gid)

	got, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Equal(t, "hello", string(got))
}

func TestServerChownExclOnExistingFile(t *testing.T) {
	client := chownClientServerPair(t)
	name := existingFile(t)
	before := ctimeOf(t, name)

	_, err := client.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	require.Error(t, err)

	require.Equal(t, before, ctimeOf(t, name))
}
