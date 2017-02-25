// +build darwin dragonfly freebsd !android,linux netbsd openbsd solaris
// +build cgo

package sftp

import "syscall"

func fakeFileInfoSys() interface{} {
	return &syscall.Stat_t{Uid: 65534, Gid: 65534}
}
