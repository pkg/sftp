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
	case syscall.EPERM:
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

// toFileMode converts sftp filemode bits to the os.FileMode specification
func toFileMode(mode sshfx.FileMode) os.FileMode {
	var ret = os.FileMode(mode & sshfx.ModePerm)

	switch mode & sshfx.ModeType {
	case sshfx.ModeNamedPipe:
		ret |= os.ModeNamedPipe
	case sshfx.ModeCharDevice:
		ret |= os.ModeDevice | os.ModeCharDevice
	case sshfx.ModeDir:
		ret |= os.ModeDir
	case sshfx.ModeDevice:
		ret |= os.ModeDevice
	case sshfx.ModeRegular:
		// nothing to do
	case sshfx.ModeSymlink:
		ret |= os.ModeSymlink
	case sshfx.ModeSocket:
		ret |= os.ModeSocket
	default:
		ret |= os.ModeIrregular
	}

	if mode&sshfx.ModeSetUID != 0 {
		ret |= os.ModeSetuid
	}
	if mode&sshfx.ModeSetGID != 0 {
		ret |= os.ModeSetgid
	}
	if mode&sshfx.ModeSticky != 0 {
		ret |= os.ModeSticky
	}

	return ret
}

// fromFileMode converts from the os.FileMode specification to sftp filemode bits
func fromFileMode(mode os.FileMode) sshfx.FileMode {
	ret := sshfx.FileMode(mode & os.ModePerm)

	switch {
	case mode&os.ModeType == 0:
		ret |= sshfx.ModeRegular
	case mode&os.ModeDir != 0:
		ret |= sshfx.ModeDir
	case mode&os.ModeSymlink != 0:
		ret |= sshfx.ModeSymlink
	case mode&os.ModeDevice != 0:
		if mode&os.ModeCharDevice != 0 {
			ret |= sshfx.ModeCharDevice
		} else {
			ret |= sshfx.ModeDevice
		}
	case mode&os.ModeNamedPipe != 0:
		ret |= sshfx.ModeNamedPipe
	case mode&os.ModeSocket != 0:
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

	return ret
}
