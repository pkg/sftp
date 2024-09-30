//go:build !windows
// +build !windows

package localfs

import (
	"os"
)

func posixRename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
