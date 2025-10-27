package sftp

import (
	"io"
	"io/fs"
	"testing"
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
