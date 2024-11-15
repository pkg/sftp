package sftp

import (
	"errors"
	"io/fs"
	"syscall"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

// translateErrorString translates a syscall error number to a SFTP error code.
func translateErrorString(errno syscall.ErrorString) sshfx.Status {
	switch errno {
	case "":
		return sshfx.StatusOK
	case syscall.ENOENT:
		return sshfx.StatusNoSuchFile
	case syscall.EACCES, syscall.EPERM:
		return sshfx.StatusPermissionDenied
	case syscall.EPLAN9:
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

	var errno syscall.ErrorString
	if errors.As(err, &errno) {
		pkt.StatusCode = translateErrorString(errno)
		return true
	}

	return false
}
