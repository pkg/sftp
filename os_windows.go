package sftp

import "syscall"

// fakeFileInfoSys returns a platform dependent value normally
// returned from the os#FileInfo.Sys method.
func fakeFileInfoSys() interface{} {
	return syscall.Win32FileAttributeData{}
}
