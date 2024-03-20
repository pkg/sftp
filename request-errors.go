package sftp

type fxerr uint32

// Error types that match the SFTP's SSH_FXP_STATUS codes. Gives you more
// direct control of the errors being sent vs. letting the library work them
// out from the standard os/io errors.
const (
	ErrSSHFxOk                      = fxerr(sshFxOk)
	ErrSSHFxEOF                     = fxerr(sshFxEOF)
	ErrSSHFxNoSuchFile              = fxerr(sshFxNoSuchFile)
	ErrSSHFxPermissionDenied        = fxerr(sshFxPermissionDenied)
	ErrSSHFxFailure                 = fxerr(sshFxFailure)
	ErrSSHFxBadMessage              = fxerr(sshFxBadMessage)
	ErrSSHFxNoConnection            = fxerr(sshFxNoConnection)
	ErrSSHFxConnectionLost          = fxerr(sshFxConnectionLost)
	ErrSSHFxOpUnsupported           = fxerr(sshFxOPUnsupported)
	ErrSSHFxInvalidHandle           = fxerr(sshFxInvalidHandle)
	ErrSSHFxNoSuchPath              = fxerr(sshFxNoSuchPath)
	ErrSSHFxFileAlreadyExists       = fxerr(sshFxFileAlreadyExists)
	ErrSSHFxWriteProtect            = fxerr(sshFxWriteProtect)
	ErrSSHFxNoMedia                 = fxerr(sshFxNoMedia)
	ErrSSHFxNoSpaceOnFilesystem     = fxerr(sshFxNoSpaceOnFilesystem)
	ErrSSHFxQuotaExceeded           = fxerr(sshFxQuotaExceeded)
	ErrSSHFxUnknownPrincipal        = fxerr(sshFxUnknownPrincipal)
	ErrSSHFxLockConflict            = fxerr(sshFxLockConflict)
	ErrSSHFxDirNotEmpty             = fxerr(sshFxDirNotEmpty)
	ErrSSHFxNotADirectory           = fxerr(sshFxNotADirectory)
	ErrSSHFxInvalidFilename         = fxerr(sshFxInvalidFilename)
	ErrSSHFxLinkLoop                = fxerr(sshFxLinkLoop)
	ErrSSHFxCannotDelete            = fxerr(sshFxCannotDelete)
	ErrSSHFxInvalidParameter        = fxerr(sshFxInvalidParameter)
	ErrSSHFxFileIsADirectory        = fxerr(sshFxFileIsADirectory)
	ErrSSHFxByteRangeLockConflict   = fxerr(sshFxByteRangeLockConflict)
	ErrSSHFxByteRangeLockRefused    = fxerr(sshFxByteRangeLockRefused)
	ErrSSHFxDeletePending           = fxerr(sshFxDeletePending)
	ErrSSHFxFileCorrupt             = fxerr(sshFxFileCorrupt)
	ErrSSHFxOwnerInvalid            = fxerr(sshFxOwnerInvalid)
	ErrSSHFxGroupInvalid            = fxerr(sshFxGroupInvalid)
	ErrSSHFxNoMatchingByteRangeLock = fxerr(sshFxNoMatchingByteRangeLock)
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
	case ErrSSHFxInvalidHandle:
		return "invalid handle"
	case ErrSSHFxNoSuchPath:
		return "no such path"
	case ErrSSHFxFileAlreadyExists:
		return "file already exists"
	case ErrSSHFxWriteProtect:
		return "write protect"
	case ErrSSHFxNoMedia:
		return "no media"
	case ErrSSHFxNoSpaceOnFilesystem:
		return "no space on filesystem"
	case ErrSSHFxQuotaExceeded:
		return "quota exceeded"
	case ErrSSHFxUnknownPrincipal:
		return "unknown principal"
	case ErrSSHFxLockConflict:
		return "lock conflict"
	case ErrSSHFxDirNotEmpty:
		return "dir not empty"
	case ErrSSHFxNotADirectory:
		return "not a directory"
	case ErrSSHFxInvalidFilename:
		return "invalid filename"
	case ErrSSHFxLinkLoop:
		return "link loop"
	case ErrSSHFxCannotDelete:
		return "cannot delete"
	case ErrSSHFxInvalidParameter:
		return "invalid parameter"
	case ErrSSHFxFileIsADirectory:
		return "file is a directory"
	case ErrSSHFxByteRangeLockConflict:
		return "byte range lock conflict"
	case ErrSSHFxByteRangeLockRefused:
		return "byte range lock refused"
	case ErrSSHFxDeletePending:
		return "delete pending"
	case ErrSSHFxFileCorrupt:
		return "file corrput"
	case ErrSSHFxOwnerInvalid:
		return "owner invalid"
	case ErrSSHFxGroupInvalid:
		return "group invalid"
	case ErrSSHFxNoMatchingByteRangeLock:
		return "no matching byte range lock"
	default:
		return "failure"
	}
}
