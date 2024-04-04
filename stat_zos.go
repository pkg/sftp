//go:build zos
// +build zos

package sftp

import (
	"os"
	"syscall"
)

const EBADF = syscall.EBADF

// z/OS syscall constants are not compatible with the stat structure returned
// from the server. Define the OpenBSD ones here instead.
const (
	S_IFMT   = 0xF000
	S_IFIFO  = 0x1000
	S_IFCHR  = 0x2000
	S_IFDIR  = 0x4000
	S_IFBLK  = 0x6000
	S_IFREG  = 0x8000
	S_IFLNK  = 0xA000
	S_IFSOCK = 0xC000
)

func wrapPathError(filepath string, err error) error {
	if errno, ok := err.(syscall.Errno); ok {
		return &os.PathError{Path: filepath, Err: errno}
	}
	return err
}

// translateErrno translates a syscall error number to a SFTP error code.
func translateErrno(errno syscall.Errno) uint32 {
	switch errno {
	case 0:
		return sshFxOk
	case syscall.ENOENT:
		return sshFxNoSuchFile
	case syscall.EACCES, syscall.EPERM:
		return sshFxPermissionDenied
	}

	return sshFxFailure
}

func translateSyscallError(err error) (uint32, bool) {
	switch e := err.(type) {
	case syscall.Errno:
		return translateErrno(e), true
	case *os.PathError:
		debug("statusFromError,pathError: error is %T %#v", e.Err, e.Err)
		if errno, ok := e.Err.(syscall.Errno); ok {
			return translateErrno(errno), true
		}
	}
	return 0, false
}

// isRegular returns true if the mode describes a regular file.
func isRegular(mode uint32) bool {
	return mode&S_IFMT == S_IFREG
}

// toFileMode converts sftp filemode bits to the os.FileMode specification
func toFileMode(mode uint32) os.FileMode {
	var fm = os.FileMode(mode & 0777)

	switch mode & S_IFMT {
	case S_IFBLK:
		fm |= os.ModeDevice
	case S_IFCHR:
		fm |= os.ModeDevice | os.ModeCharDevice
	case S_IFDIR:
		fm |= os.ModeDir
	case S_IFIFO:
		fm |= os.ModeNamedPipe
	case S_IFLNK:
		fm |= os.ModeSymlink
	case S_IFREG:
		// nothing to do
	case S_IFSOCK:
		fm |= os.ModeSocket
	}

	if mode&syscall.S_ISUID != 0 {
		fm |= os.ModeSetuid
	}
	if mode&syscall.S_ISGID != 0 {
		fm |= os.ModeSetgid
	}
	if mode&syscall.S_ISVTX != 0 {
		fm |= os.ModeSticky
	}

	return fm
}

// fromFileMode converts from the os.FileMode specification to sftp filemode bits
func fromFileMode(mode os.FileMode) uint32 {
	ret := uint32(mode & os.ModePerm)

	switch mode & os.ModeType {
	case os.ModeDevice | os.ModeCharDevice:
		ret |= S_IFCHR
	case os.ModeDevice:
		ret |= S_IFBLK
	case os.ModeDir:
		ret |= S_IFDIR
	case os.ModeNamedPipe:
		ret |= S_IFIFO
	case os.ModeSymlink:
		ret |= S_IFLNK
	case 0:
		ret |= S_IFREG
	case os.ModeSocket:
		ret |= S_IFSOCK
	}

	if mode&os.ModeSetuid != 0 {
		ret |= syscall.S_ISUID
	}
	if mode&os.ModeSetgid != 0 {
		ret |= syscall.S_ISGID
	}
	if mode&os.ModeSticky != 0 {
		ret |= syscall.S_ISVTX
	}

	return ret
}

const (
	s_ISUID = syscall.S_ISUID
	s_ISGID = syscall.S_ISGID
	s_ISVTX = syscall.S_ISVTX
)
