//go:build !windows
// +build !windows

package localfs

import (
	"fmt"
	"io/fs"
	"os"
)

func (h *ServerHandler) openfile(path string, flag int, mod fs.FileMode) (*File, error) {
	f, err := os.OpenFile(path, flag, mod)
	if err != nil {
		return nil, err
	}

	return &File{
		File: f,
		srv:  h,

		filename: path,
		handle:   fmt.Sprint(h.handles.Add(1)),
	}, nil
}
