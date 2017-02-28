// +build !cgo,!plan9 windows android

package sftp

import "syscall"

func fakeFileInfoSys() interface{} {
	return syscall.Win32FileAttributeData{}
}
