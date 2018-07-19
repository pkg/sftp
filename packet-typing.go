package sftp

import (
	"encoding"

	"github.com/pkg/errors"
)

// all incoming packets
type RequestPacket interface {
	encoding.BinaryUnmarshaler
	Id() uint32
}

type requestChan chan RequestPacket

type ResponsePacket interface {
	encoding.BinaryMarshaler
	Id() uint32
}

// interfaces to group types
type HasPath interface {
	RequestPacket
	GetPath() string
}

type HasHandle interface {
	RequestPacket
	GetHandle() string
}

type NotReadOnly interface {
	NotReadOnly()
}

// take raw incoming packet data and build packet objects
func makePacket(p rxPacket) (pkt RequestPacket, err error) {
	switch p.pktType {
	case ssh_FXP_INIT:
		pkt = &SSHFxInitPacket{}
	case ssh_FXP_LSTAT:
		pkt = &SSHFxpLstatPacket{}
	case ssh_FXP_OPEN:
		pkt = &SSHFxpOpenPacket{}
	case ssh_FXP_CLOSE:
		pkt = &SSHFxpClosePacket{}
	case ssh_FXP_READ:
		pkt = &SSHFxpReadPacket{}
	case ssh_FXP_WRITE:
		pkt = &SSHFxpWritePacket{}
	case ssh_FXP_FSTAT:
		pkt = &SSHFxpFstatPacket{}
	case ssh_FXP_SETSTAT:
		pkt = &SSHFxpSetstatPacket{}
	case ssh_FXP_FSETSTAT:
		pkt = &SSHFxpFsetstatPacket{}
	case ssh_FXP_OPENDIR:
		pkt = &SSHFxpOpendirPacket{}
	case ssh_FXP_READDIR:
		pkt = &SSHFxpReaddirPacket{}
	case ssh_FXP_REMOVE:
		pkt = &SSHFxpRemovePacket{}
	case ssh_FXP_MKDIR:
		pkt = &SSHFxpMkdirPacket{}
	case ssh_FXP_RMDIR:
		pkt = &SSHFxpRmdirPacket{}
	case ssh_FXP_REALPATH:
		pkt = &SSHFxpRealpathPacket{}
	case ssh_FXP_STAT:
		pkt = &SSHFxpStatPacket{}
	case ssh_FXP_RENAME:
		pkt = &SSHFxpRenamePacket{}
	case ssh_FXP_READLINK:
		pkt = &SSHFxpReadlinkPacket{}
	case ssh_FXP_SYMLINK:
		pkt = &SSHFxpSymlinkPacket{}
	case ssh_FXP_EXTENDED:
		pkt = &SSHFxpExtendedPacket{}
	default:
		err = errors.Errorf("unhandled packet type: %s", p.pktType)
		return
	}
	// Return partially unpacked packet to allow callers to return
	// error messages appropriately with necessary id() method.
	err = pkt.UnmarshalBinary(p.pktBytes)
	return
}
