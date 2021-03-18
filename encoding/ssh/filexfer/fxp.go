package filexfer

import (
	"fmt"
)

// PacketType defines the various SFTP packet types.
type PacketType uint8

// Request packet types.
const (
	PacketTypeInit = PacketType(iota + 1)
	PacketTypeVersion
	PacketTypeOpen
	PacketTypeClose
	PacketTypeRead
	PacketTypeWrite
	PacketTypeLstat
	PacketTypeFstat
	PacketTypeSetstat
	PacketTypeFsetstat
	PacketTypeOpendir
	PacketTypeReaddir
	PacketTypeRemove
	PacketTypeMkdir
	PacketTypeRmdir
	PacketTypeRealpath
	PacketTypeStat
	PacketTypeRename
	PacketTypeReadlink
	PacketTypeSymlink

	// see draft-ietf-secsh-filexfer-13
	// https://tools.ietf.org/html/draft-ietf-secsh-filexfer-13#section-4.3
	// Defined only for interoperability!
	PacketTypeLink
	PacketTypeBlock
	PacketTypeUnblock
)

// Response packet types.
const (
	PacketTypeStatus = PacketType(iota + 101)
	PacketTypeHandle
	PacketTypeData
	PacketTypeName
	PacketTypeAttrs
)

// Extended packet types.
const (
	PacketTypeExtended = PacketType(iota + 200)
	PacketTypeExtendedReply
)

func (f PacketType) String() string {
	switch f {
	case PacketTypeInit:
		return "SSH_FXP_INIT"
	case PacketTypeVersion:
		return "SSH_FXP_VERSION"
	case PacketTypeOpen:
		return "SSH_FXP_OPEN"
	case PacketTypeClose:
		return "SSH_FXP_CLOSE"
	case PacketTypeRead:
		return "SSH_FXP_READ"
	case PacketTypeWrite:
		return "SSH_FXP_WRITE"
	case PacketTypeLstat:
		return "SSH_FXP_LSTAT"
	case PacketTypeFstat:
		return "SSH_FXP_FSTAT"
	case PacketTypeSetstat:
		return "SSH_FXP_SETSTAT"
	case PacketTypeFsetstat:
		return "SSH_FXP_FSETSTAT"
	case PacketTypeOpendir:
		return "SSH_FXP_OPENDIR"
	case PacketTypeReaddir:
		return "SSH_FXP_READDIR"
	case PacketTypeRemove:
		return "SSH_FXP_REMOVE"
	case PacketTypeMkdir:
		return "SSH_FXP_MKDIR"
	case PacketTypeRmdir:
		return "SSH_FXP_RMDIR"
	case PacketTypeRealpath:
		return "SSH_FXP_REALPATH"
	case PacketTypeStat:
		return "SSH_FXP_STAT"
	case PacketTypeRename:
		return "SSH_FXP_RENAME"
	case PacketTypeReadlink:
		return "SSH_FXP_READLINK"
	case PacketTypeSymlink:
		return "SSH_FXP_SYMLINK"
	case PacketTypeLink:
		return "SSH_FXP_LINK"
	case PacketTypeBlock:
		return "SSH_FXP_BLOCK"
	case PacketTypeUnblock:
		return "SSH_FXP_UNBLOCK"
	case PacketTypeStatus:
		return "SSH_FXP_STATUS"
	case PacketTypeHandle:
		return "SSH_FXP_HANDLE"
	case PacketTypeData:
		return "SSH_FXP_DATA"
	case PacketTypeName:
		return "SSH_FXP_NAME"
	case PacketTypeAttrs:
		return "SSH_FXP_ATTRS"
	case PacketTypeExtended:
		return "SSH_FXP_EXTENDED"
	case PacketTypeExtendedReply:
		return "SSH_FXP_EXTENDED_REPLY"
	default:
		return fmt.Sprintf("SSH_FXP_UNKNOWN(%d)", f)
	}
}
