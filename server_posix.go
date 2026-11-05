//go:build !windows
// +build !windows

package sftp

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// errNotRegularFile is returned by a Server restricted to regular files.
var errNotRegularFile = errors.New("not a regular file")

func (s *Server) openfile(path string, flag int, mode fs.FileMode) (file, error) {
	if !s.regularFilesOnly {
		return os.OpenFile(path, flag, mode)
	}

	// Refuse anything that isn't a regular file before opening it, as opening
	// a FIFO blocks and opening a device node may reach the device.
	fi, err := os.Stat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	if err == nil && !fi.Mode().IsRegular() {
		return nil, &os.PathError{Op: "open", Path: path, Err: errNotRegularFile}
	}

	// The file may have been swapped since the check, so never block on the
	// open and check what was actually opened.
	f, err := os.OpenFile(path, flag|syscall.O_NONBLOCK, mode)
	if err != nil {
		return nil, err
	}

	fi, err = f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	if !fi.Mode().IsRegular() {
		_ = f.Close()
		return nil, &os.PathError{Op: "open", Path: path, Err: errNotRegularFile}
	}

	return f, nil
}

func (s *Server) lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func (s *Server) stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}
