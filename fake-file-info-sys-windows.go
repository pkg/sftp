// +build windows

package sftp

import "syscall"

func fakeFileInfoSys() interface{} {
	return syscall.Win32FileAttributeData{}
}
