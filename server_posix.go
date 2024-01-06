//go:build !windows
// +build !windows

package sftp

import (
	"io/fs"
	"os"
)

func openFileLike(path string, flag int, mode fs.FileMode) (FileLike, error) {
	return os.OpenFile(path, flag, mode)
}
