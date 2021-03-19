package filexfer

import (
	"fmt"
)

// Status defines the SFTP error codes used in SSH_FXP_STATUS response packets.
type Status uint32

// Defines the various SSH_FX_* values.
const (
	// see draft-ietf-secsh-filexfer-02
	// https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-7
	StatusOK = Status(iota)
	StatusEOF
	StatusNoSuchFile
	StatusPermissionDenied
	StatusFailure
	StatusBadMessage
	StatusNoConnection
	StatusConnectionLost
	StatusOPUnsupported

	// see draft-ietf-secsh-filexfer-13
	// https://tools.ietf.org/html/draft-ietf-secsh-filexfer-13#section-9.1
	// Defined only for interoperability!
	StatusInvalidHandle
	StatusNoSuchPath
	StatusFileAlreadyExists
	StatusWriteProtect
	StatusNoMedia
	StatusNoSpaceOnFilesystem
	StatusQuotaExceeded
	StatusUnknownPrincipal
	StatusLockConflict
	StatusDirNotEmpty
	StatusNotADirectory
	StatusInvalidFilename
	StatusLinkLoop
	StatusCannotDelete
	StatusInvalidParameter
	StatusFileIsADirectory
	StatusByteRangeLockConflict
	StatusByteRangeLockRefused
	StatusDeletePending
	StatusFileCorrupt
	StatusOwnerInvalid
	StatusGroupInvalid
	StatusNoMatchingByteRangeLock
)

func (f Status) String() string {
	switch f {
	case StatusOK:
		return "SSH_FX_OK"
	case StatusEOF:
		return "SSH_FX_EOF"
	case StatusNoSuchFile:
		return "SSH_FX_NO_SUCH_FILE"
	case StatusPermissionDenied:
		return "SSH_FX_PERMISSION_DENIED"
	case StatusFailure:
		return "SSH_FX_FAILURE"
	case StatusBadMessage:
		return "SSH_FX_BAD_MESSAGE"
	case StatusNoConnection:
		return "SSH_FX_NO_CONNECTION"
	case StatusConnectionLost:
		return "SSH_FX_CONNECTION_LOST"
	case StatusOPUnsupported:
		return "SSH_FX_OP_UNSUPPORTED"
	case StatusInvalidHandle:
		return "SSH_FX_INVALID_HANDLE"
	case StatusNoSuchPath:
		return "SSH_FX_NO_SUCH_PATH"
	case StatusFileAlreadyExists:
		return "SSH_FX_FILE_ALREADY_EXISTS"
	case StatusWriteProtect:
		return "SSH_FX_WRITE_PROTECT"
	case StatusNoMedia:
		return "SSH_FX_NO_MEDIA"
	case StatusNoSpaceOnFilesystem:
		return "SSH_FX_NO_SPACE_ON_FILESYSTEM"
	case StatusQuotaExceeded:
		return "SSH_FX_QUOTA_EXCEEDED"
	case StatusUnknownPrincipal:
		return "SSH_FX_UNKNOWN_PRINCIPAL"
	case StatusLockConflict:
		return "SSH_FX_LOCK_CONFLICT"
	case StatusDirNotEmpty:
		return "SSH_FX_DIR_NOT_EMPTY"
	case StatusNotADirectory:
		return "SSH_FX_NOT_A_DIRECTORY"
	case StatusInvalidFilename:
		return "SSH_FX_INVALID_FILENAME"
	case StatusLinkLoop:
		return "SSH_FX_LINK_LOOP"
	case StatusCannotDelete:
		return "SSH_FX_CANNOT_DELETE"
	case StatusInvalidParameter:
		return "SSH_FX_INVALID_PARAMETER"
	case StatusFileIsADirectory:
		return "SSH_FX_FILE_IS_A_DIRECTORY"
	case StatusByteRangeLockConflict:
		return "SSH_FX_BYTE_RANGE_LOCK_CONFLICT"
	case StatusByteRangeLockRefused:
		return "SSH_FX_BYTE_RANGE_LOCK_REFUSED"
	case StatusDeletePending:
		return "SSH_FX_DELETE_PENDING"
	case StatusFileCorrupt:
		return "SSH_FX_FILE_CORRUPT"
	case StatusOwnerInvalid:
		return "SSH_FX_OWNER_INVALID"
	case StatusGroupInvalid:
		return "SSH_FX_GROUP_INVALID"
	case StatusNoMatchingByteRangeLock:
		return "SSH_FX_NO_MATCHING_BYTE_RANGE_LOCK"
	default:
		return fmt.Sprintf("SSH_FX_UNKNOWN(%d)", f)
	}
}
