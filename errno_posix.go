//go:build !plan9
// +build !plan9

package sftp

import (
	"errors"
	"io/fs"
	"os"
	"syscall"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

// translateErrno translates a syscall error number to a SFTP error code.
func translateErrno(errno syscall.Errno) sshfx.Status {
	switch errno {
	case 0:
		return sshfx.StatusOK
	case syscall.ENOENT:
		return sshfx.StatusNoSuchFile
	case syscall.EACCES, syscall.EPERM:
		return sshfx.StatusPermissionDenied
	case syscall.ENOTSUP:
		return sshfx.StatusOpUnsupported
	}

	return sshfx.StatusFailure
}

func syscallErrorAsStatus(err error, pkt *sshfx.StatusPacket) bool {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err // Replace the error with the underlying error.
		pkt.ErrorMessage = err.Error()
	}

	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		err = linkErr.Err // Replace the error with the underlying error.
		pkt.ErrorMessage = err.Error()
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		pkt.StatusCode = translateErrno(errno)
		return true
	}

	return false
}
