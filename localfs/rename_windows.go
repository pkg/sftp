package localfs

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func posixRename(oldpath, newpath string) error {
	// To be consistent with POSIX rename(),
	// if newpath is a directory and is empty
	// then delete it so that MoveFileEx will succeed.
	oldfi, err := os.Stat(oldpath)
	newfi, err2 := os.Stat(newpath)
	if err == nil && err2 == nil && oldfi.IsDir() && newfi.IsDir() {
		if err = os.Remove(newpath); err != nil {
			// If this fails, then directory wasn’t empty,
			// and the MoveFileEx below would fail anyways.
			return err
		}
	}

	from, err := syscall.UTF16PtrFromString(oldpath)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(newpath)
	if err != nil {
		return err
	}

	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_COPY_ALLOWED)
}
