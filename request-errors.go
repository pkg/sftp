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
		return "the handle value was invalid"
	case ErrSSHFxNoSuchPath:
		return "the file path does not exist or is invalid"
	case ErrSSHFxFileAlreadyExists:
		return "the file already exists"
	case ErrSSHFxWriteProtect:
		return "the file is on read-only media, or the media is write protected"
	case ErrSSHFxNoMedia:
		return "the requested operation cannot be completed because there is no media available in the drive"
	case ErrSSHFxNoSpaceOnFilesystem:
		return "the requested operation cannot be completed because there is insufficient free space on the filesystem"
	case ErrSSHFxQuotaExceeded:
		return "the operation cannot be completed because it would exceed the user's storage quota"
	case ErrSSHFxUnknownPrincipal:
		return "a principal referenced by the request (either the 'owner', 'group', or 'who' field of an ACL), was unknown"
	case ErrSSHFxLockConflict:
		return "the file could not be opened because it is locked by another process"
	case ErrSSHFxDirNotEmpty:
		return "the directory is not empty"
	case ErrSSHFxNotADirectory:
		return "the specified file is not a directory"
	case ErrSSHFxInvalidFilename:
		return "the filename is not valid"
	case ErrSSHFxLinkLoop:
		return "too many symbolic links encountered or, an SSH_FXF_NOFOLLOW open encountered a symbolic link as the final component"
	case ErrSSHFxCannotDelete:
		return "the file cannot be deleted"
	case ErrSSHFxInvalidParameter:
		return "one of the parameters was out of range, or the parameters specified cannot be used together"
	case ErrSSHFxFileIsADirectory:
		return "the specified file was a directory in a context where a directory cannot be used"
	case ErrSSHFxByteRangeLockConflict:
		return "a read or write operation failed because another process's mandatory byte-range lock overlaps with the request"
	case ErrSSHFxByteRangeLockRefused:
		return "a request for a byte range lock was refused"
	case ErrSSHFxDeletePending:
		return "an operation was attempted on a file for which a delete operation is pending"
	case ErrSSHFxFileCorrupt:
		return "the file is corrupt; an filesystem integrity check should be run"
	case ErrSSHFxOwnerInvalid:
		return "the principal specified can not be assigned as an owner of a file"
	case ErrSSHFxGroupInvalid:
		return "the principal specified can not be assigned as the primary group of a file"
	case ErrSSHFxNoMatchingByteRangeLock:
		return "the requested operation could not be completed because the specified byte range lock has not been granted"
	default:
		return "failure"
	}
}
