package sshfx

import (
	"fmt"

	"github.com/pkg/sftp/v2/internal/sync"
)

// PacketType defines the various SFTP packet types.
type PacketType uint8

// Request packet types.
const (
	// https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-02.txt#section-3
	PacketTypeInit = PacketType(iota + 1)
	PacketTypeVersion
	PacketTypeOpen
	PacketTypeClose
	PacketTypeRead
	PacketTypeWrite
	PacketTypeLStat
	PacketTypeFStat
	PacketTypeSetStat
	PacketTypeFSetStat
	PacketTypeOpenDir
	PacketTypeReadDir
	PacketTypeRemove
	PacketTypeMkdir
	PacketTypeRmdir
	PacketTypeRealPath
	PacketTypeStat
	PacketTypeRename
	PacketTypeReadLink
	PacketTypeSymlink

	// https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-07.txt#section-3.3
	PacketTypeV6Link

	// https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-08.txt#section-3.3
	PacketTypeV6Block
	PacketTypeV6Unblock
)

// Response packet types.
const (
	// https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-02.txt#section-3
	PacketTypeStatus = PacketType(iota + 101)
	PacketTypeHandle
	PacketTypeData
	PacketTypeName
	PacketTypeAttrs
)

// Extended packet types.
const (
	// https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-02.txt#section-3
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
	case PacketTypeLStat:
		return "SSH_FXP_LSTAT"
	case PacketTypeFStat:
		return "SSH_FXP_FSTAT"
	case PacketTypeSetStat:
		return "SSH_FXP_SETSTAT"
	case PacketTypeFSetStat:
		return "SSH_FXP_FSETSTAT"
	case PacketTypeOpenDir:
		return "SSH_FXP_OPENDIR"
	case PacketTypeReadDir:
		return "SSH_FXP_READDIR"
	case PacketTypeRemove:
		return "SSH_FXP_REMOVE"
	case PacketTypeMkdir:
		return "SSH_FXP_MKDIR"
	case PacketTypeRmdir:
		return "SSH_FXP_RMDIR"
	case PacketTypeRealPath:
		return "SSH_FXP_REALPATH"
	case PacketTypeStat:
		return "SSH_FXP_STAT"
	case PacketTypeRename:
		return "SSH_FXP_RENAME"
	case PacketTypeReadLink:
		return "SSH_FXP_READLINK"
	case PacketTypeSymlink:
		return "SSH_FXP_SYMLINK"
	case PacketTypeV6Link:
		return "SSH_FXP_LINK"
	case PacketTypeV6Block:
		return "SSH_FXP_BLOCK"
	case PacketTypeV6Unblock:
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

var (
	readPool   = sync.NewPool[ReadPacket](64)
	writePool  = sync.NewPool[WritePacket](64)
	wrDataPool = sync.NewSlicePool[[]byte](64, DefaultMaxDataLength)
)

// PoolReturn adds a packet to an internal pool for its type, if one exists.
// If a pool has not been setup, then it is a no-op.
//
// Currently, this is only setup for [ReadPacket] and [WritePacket],
// as these are generally the most heavily used packet types.
func PoolReturn(p Packet) {
	switch p := p.(type) {
	case *ReadPacket:
		readPool.Put(p)
	case *WritePacket:
		wrDataPool.Put(p.Data)
		writePool.Put(p)
	}
}

func newRequestPacketFromType(typ PacketType) (Packet, error) {
	switch typ {
	case PacketTypeOpen:
		return new(OpenPacket), nil
	case PacketTypeClose:
		return new(ClosePacket), nil
	case PacketTypeRead:
		return readPool.Get(), nil
	case PacketTypeWrite:
		return writePool.Get(), nil
	case PacketTypeLStat:
		return new(LStatPacket), nil
	case PacketTypeFStat:
		return new(FStatPacket), nil
	case PacketTypeSetStat:
		return new(SetStatPacket), nil
	case PacketTypeFSetStat:
		return new(FSetStatPacket), nil
	case PacketTypeOpenDir:
		return new(OpenDirPacket), nil
	case PacketTypeReadDir:
		return new(ReadDirPacket), nil
	case PacketTypeRemove:
		return new(RemovePacket), nil
	case PacketTypeMkdir:
		return new(MkdirPacket), nil
	case PacketTypeRmdir:
		return new(RmdirPacket), nil
	case PacketTypeRealPath:
		return new(RealPathPacket), nil
	case PacketTypeStat:
		return new(StatPacket), nil
	case PacketTypeRename:
		return new(RenamePacket), nil
	case PacketTypeReadLink:
		return new(ReadLinkPacket), nil
	case PacketTypeSymlink:
		return new(SymlinkPacket), nil
	case PacketTypeExtended:
		return new(ExtendedPacket), nil
	default:
		return nil, &StatusPacket{
			StatusCode:   StatusBadMessage,
			ErrorMessage: fmt.Sprintf("invalid packet type: %v", typ),
		}
	}
}

func newPacketFromType(typ PacketType) (Packet, error) {
	switch typ {
	case PacketTypeStatus:
		return new(StatusPacket), nil
	case PacketTypeHandle:
		return new(HandlePacket), nil
	case PacketTypeData:
		return new(DataPacket), nil
	case PacketTypeName:
		return new(NamePacket), nil
	case PacketTypeAttrs:
		return new(AttrsPacket), nil
	case PacketTypeExtendedReply:
		return new(ExtendedReplyPacket), nil
	}

	return newRequestPacketFromType(typ)
}
