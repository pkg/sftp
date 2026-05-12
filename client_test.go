package sftp

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"testing"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

func TestClient(t *testing.T) {
	type allFile interface {
		fs.File

		// Is it impossible to implement this properly?
		// It is a protocol error to attempt to use an ordinary file handle returned by SSH_FXP_OPEN.
		// And, fs.FS only permits a simple `Open()`.
		// fs.ReadDirFile
	}

	var _ allFile = new(File)

	type allDir interface {
		io.Closer
		ReadDir(n int) ([]fs.DirEntry, error)
	}

	var _ allDir = new(Dir)

	type allFS interface {
		fs.FS
		// fs.GlobFS
		fs.ReadDirFS
		fs.ReadFileFS
		fs.StatFS
		fs.SubFS
	}

	var _ allFS = new(fsys)
}

type sink struct{}

func (sink) Close() error { return nil }
func (sink) Write(p []byte) (int, error) { return len(p), nil }

// Issue #418: panic in clientConn.recv when the sid is incomplete.
func TestClientNoSid(t *testing.T) {
	initPkt := &sshfx.VersionPacket{
		Version: sftpProtocolVersion,
	}

	initData, err := initPkt.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	stream := new(bytes.Buffer)
	stream.Write(initData)
	stream.Write([]byte{ 0, 0, 0, 10, 0, 0 })

	cl, err := NewClientPipe(t.Context(), stream, sink{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = cl.Stat("anything")
	if !errors.Is(err, sshfx.StatusConnectionLost) {
		t.Errorf("cl.Stat = %v, expected sshfx.StatusConnectionLost", err)
	}
}
