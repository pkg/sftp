//go:build !plan9
// +build !plan9

package sftp

import (
	"os"
	"syscall"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

const EBADF = syscall.EBADF

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
	return sshfx.FileMode(mode)&sshfx.ModeType == sshfx.ModeRegular
}

// toFileMode converts sftp filemode bits to the os.FileMode specification
func toFileMode(mode uint32) os.FileMode {
	var fm = os.FileMode(mode & 0777)

	switch sshfx.FileMode(mode) & sshfx.ModeType {
	case sshfx.ModeDevice:
		fm |= os.ModeDevice
	case sshfx.ModeCharDevice:
		fm |= os.ModeDevice | os.ModeCharDevice
	case sshfx.ModeDir:
		fm |= os.ModeDir
	case sshfx.ModeNamedPipe:
		fm |= os.ModeNamedPipe
	case sshfx.ModeSymlink:
		fm |= os.ModeSymlink
	case sshfx.ModeRegular:
		// nothing to do
	case sshfx.ModeSocket:
		fm |= os.ModeSocket
	}

	if sshfx.FileMode(mode)&sshfx.ModeSetUID != 0 {
		fm |= os.ModeSetuid
	}
	if sshfx.FileMode(mode)&sshfx.ModeSetGID != 0 {
		fm |= os.ModeSetgid
	}
	if sshfx.FileMode(mode)&sshfx.ModeSticky != 0 {
		fm |= os.ModeSticky
	}

	return fm
}

// fromFileMode converts from the os.FileMode specification to sftp filemode bits
func fromFileMode(mode os.FileMode) uint32 {
	ret := sshfx.FileMode(mode & os.ModePerm)

	switch mode & os.ModeType {
	case os.ModeDevice | os.ModeCharDevice:
		ret |= sshfx.ModeCharDevice
	case os.ModeDevice:
		ret |= sshfx.ModeDevice
	case os.ModeDir:
		ret |= sshfx.ModeDir
	case os.ModeNamedPipe:
		ret |= sshfx.ModeNamedPipe
	case os.ModeSymlink:
		ret |= sshfx.ModeSymlink
	case 0:
		ret |= sshfx.ModeRegular
	case os.ModeSocket:
		ret |= sshfx.ModeSocket
	}

	if mode&os.ModeSetuid != 0 {
		ret |= sshfx.ModeSetUID
	}
	if mode&os.ModeSetgid != 0 {
		ret |= sshfx.ModeSetGID
	}
	if mode&os.ModeSticky != 0 {
		ret |= sshfx.ModeSticky
	}

	return uint32(ret)
}

const (
	s_ISUID = uint32(sshfx.ModeSetUID)
	s_ISGID = uint32(sshfx.ModeSetGID)
	s_ISVTX = uint32(sshfx.ModeSticky)
)
