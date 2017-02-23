//+build !windows

package sftp

import "syscall"

// fakeFileInfoSys returns a platform dependent value normally
// returned from the os#FileInfo.Sys method.
func fakeFileInfoSys() interface{} {
	return &syscall.Stat_t{Uid: 65534, Gid: 65534}
}
