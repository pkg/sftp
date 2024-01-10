//go:build !windows
// +build !windows

package sftp

import (
	"io/fs"
	"os"
)

func (s *Server) openfile(path string, flag int, mode fs.FileMode) (file, error) {
	return os.OpenFile(path, flag, mode)
}
