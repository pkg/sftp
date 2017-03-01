// +build !windows

package sftp

import "syscall"

func fakeFileInfoSys() interface{} {
	return &syscall.Stat_t{Uid: 65534, Gid: 65534}
}
