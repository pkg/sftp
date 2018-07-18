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

//// define types by adding methods
// hasPath
func (p SSHFxpLstatPacket) GetPath() string    { return p.Path }
func (p SSHFxpStatPacket) GetPath() string     { return p.Path }
func (p SSHFxpRmdirPacket) GetPath() string    { return p.Path }
func (p SSHFxpReadlinkPacket) GetPath() string { return p.Path }
func (p SSHFxpRealpathPacket) GetPath() string { return p.Path }
func (p SSHFxpMkdirPacket) GetPath() string    { return p.Path }
func (p SSHFxpSetstatPacket) GetPath() string  { return p.Path }
func (p SSHFxpStatvfsPacket) GetPath() string  { return p.Path }
func (p SSHFxpRemovePacket) GetPath() string   { return p.Filename }
func (p SSHFxpRenamePacket) GetPath() string   { return p.Oldpath }
func (p SSHFxpSymlinkPacket) GetPath() string  { return p.Targetpath }
func (p SSHFxpOpendirPacket) GetPath() string  { return p.Path }
func (p SSHFxpOpenPacket) GetPath() string     { return p.Path }

func (p SSHFxpExtendedPacketPosixRename) GetPath() string { return p.Oldpath }

// hasHandle
func (p SSHFxpFstatPacket) GetHandle() string    { return p.Handle }
func (p SSHFxpFsetstatPacket) GetHandle() string { return p.Handle }
func (p SSHFxpReadPacket) GetHandle() string     { return p.Handle }
func (p SSHFxpWritePacket) GetHandle() string    { return p.Handle }
func (p SSHFxpReaddirPacket) GetHandle() string  { return p.Handle }
func (p SSHFxpClosePacket) GetHandle() string    { return p.Handle }

// NotReadOnly
func (p SSHFxpWritePacket) NotReadOnly()               {}
func (p SSHFxpSetstatPacket) NotReadOnly()             {}
func (p SSHFxpFsetstatPacket) NotReadOnly()            {}
func (p SSHFxpRemovePacket) NotReadOnly()              {}
func (p SSHFxpMkdirPacket) NotReadOnly()               {}
func (p SSHFxpRmdirPacket) NotReadOnly()               {}
func (p SSHFxpRenamePacket) NotReadOnly()              {}
func (p SSHFxpSymlinkPacket) NotReadOnly()             {}
func (p SSHFxpExtendedPacketPosixRename) NotReadOnly() {}

// some packets with ID are missing id()
func (p SSHFxpDataPacket) Id() uint32   { return p.ID }
func (p SSHFxpStatusPacket) Id() uint32 { return p.ID }
func (p SSHFxpStatResponse) Id() uint32 { return p.ID }
func (p SSHFxpNamePacket) Id() uint32   { return p.ID }
func (p SSHFxpHandlePacket) Id() uint32 { return p.ID }
func (p SSHFxVersionPacket) Id() uint32 { return 0 }

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
