package sftp

import (
	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

type fxerr uint32

// Error types that match the SFTP's SSH_FXP_STATUS codes. Gives you more
// direct control of the errors being sent vs. letting the library work them
// out from the standard os/io errors.
const (
	ErrSSHFxOk               = fxerr(sshFxOk)
	ErrSSHFxEOF              = fxerr(sshFxEOF)
	ErrSSHFxNoSuchFile       = fxerr(sshFxNoSuchFile)
	ErrSSHFxPermissionDenied = fxerr(sshFxPermissionDenied)
	ErrSSHFxFailure          = fxerr(sshFxFailure)
	ErrSSHFxBadMessage       = fxerr(sshFxBadMessage)
	ErrSSHFxNoConnection     = fxerr(sshFxNoConnection)
	ErrSSHFxConnectionLost   = fxerr(sshFxConnectionLost)
	ErrSSHFxOpUnsupported    = fxerr(sshFxOPUnsupported)
)

// Deprecated error types, these are aliases for the new ones, please use the new ones directly
const (
	ErrSshFxOk               = ErrSSHFxOk
	ErrSshFxEof              = ErrSSHFxEOF
	ErrSshFxNoSuchFile       = ErrSSHFxNoSuchFile
	ErrSshFxPermissionDenied = ErrSSHFxPermissionDenied
	ErrSshFxFailure          = ErrSSHFxFailure
	ErrSshFxBadMessage       = ErrSSHFxBadMessage
	ErrSshFxNoConnection     = ErrSSHFxNoConnection
	ErrSshFxConnectionLost   = ErrSSHFxConnectionLost
	ErrSshFxOpUnsupported    = ErrSSHFxOpUnsupported
)

func (e fxerr) Error() string {
	switch e {
	case ErrSSHFxOk:
		return "OK"
	case ErrSSHFxEOF:
		return "EOF"
	case ErrSSHFxNoSuchFile:
		return "no such file"
	case ErrSSHFxPermissionDenied:
		return "permission denied"
	case ErrSSHFxBadMessage:
		return "bad message"
	case ErrSSHFxNoConnection:
		return "no connection"
	case ErrSSHFxConnectionLost:
		return "connection lost"
	case ErrSSHFxOpUnsupported:
		return "operation unsupported"
	default:
		return "failure"
	}
}

// Is returns true if target represents the same SSH_FX_code as this fxerr.
func (e fxerr) Is(target error) bool {
	switch target := target.(type) {
	case fxerr:
		return e == target
	case *StatusError:
		return e == fxerr(target.Code)
	// The errors below cannot be commutatively equivalent.
	case sshfx.Status:
		return sshfx.Status(e) == target
	case *sshfx.StatusPacket:
		return sshfx.Status(e) == target.StatusCode
	}

	return false
}
