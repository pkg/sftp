package sftp

import (
	"encoding"

	"github.com/pkg/errors"
)

// all incoming packets
type packet interface {
	encoding.BinaryUnmarshaler
	id() uint32
}

// interfaces to group types
type hasPath interface {
	getPath() string
}

type hasHandle interface {
	getHandle() string
}

//// define types by adding methods
// hasPath
func (p sshFxpOpendirPacket) getPath() string  { return p.Path }
func (p sshFxpLstatPacket) getPath() string    { return p.Path }
func (p sshFxpStatPacket) getPath() string     { return p.Path }
func (p sshFxpRmdirPacket) getPath() string    { return p.Path }
func (p sshFxpReadlinkPacket) getPath() string { return p.Path }
func (p sshFxpRealpathPacket) getPath() string { return p.Path }
func (p sshFxpOpenPacket) getPath() string     { return p.Path }
func (p sshFxpMkdirPacket) getPath() string    { return p.Path }
func (p sshFxpSetstatPacket) getPath() string  { return p.Path }
func (p sshFxpStatvfsPacket) getPath() string  { return p.Path }
func (p sshFxpRemovePacket) getPath() string   { return p.Filename }
func (p sshFxpRenamePacket) getPath() string   { return p.Oldpath }
func (p sshFxpSymlinkPacket) getPath() string  { return p.Targetpath }

// hasHandle
func (p sshFxpFstatPacket) getHandle() string    { return p.Handle }
func (p sshFxpFsetstatPacket) getHandle() string { return p.Handle }
func (p sshFxpReadPacket) getHandle() string     { return p.Handle }
func (p sshFxpWritePacket) getHandle() string    { return p.Handle }
func (p sshFxpReaddirPacket) getHandle() string  { return p.Handle }

// this has a handle, but is only used for close
func (p sshFxpClosePacket) getHandle() string { return p.Handle }

// take raw incoming packet data and build packet objects
func makePacket(p rxPacket) (packet, error) {
	var pkt packet
	switch p.pktType {
	case ssh_FXP_INIT:
		pkt = &sshFxInitPacket{}
	case ssh_FXP_LSTAT:
		pkt = &sshFxpLstatPacket{}
	case ssh_FXP_OPEN:
		pkt = &sshFxpOpenPacket{}
	case ssh_FXP_CLOSE:
		pkt = &sshFxpClosePacket{}
	case ssh_FXP_READ:
		pkt = &sshFxpReadPacket{}
	case ssh_FXP_WRITE:
		pkt = &sshFxpWritePacket{}
	case ssh_FXP_FSTAT:
		pkt = &sshFxpFstatPacket{}
	case ssh_FXP_SETSTAT:
		pkt = &sshFxpSetstatPacket{}
	case ssh_FXP_FSETSTAT:
		pkt = &sshFxpFsetstatPacket{}
	case ssh_FXP_OPENDIR:
		pkt = &sshFxpOpendirPacket{}
	case ssh_FXP_READDIR:
		pkt = &sshFxpReaddirPacket{}
	case ssh_FXP_REMOVE:
		pkt = &sshFxpRemovePacket{}
	case ssh_FXP_MKDIR:
		pkt = &sshFxpMkdirPacket{}
	case ssh_FXP_RMDIR:
		pkt = &sshFxpRmdirPacket{}
	case ssh_FXP_REALPATH:
		pkt = &sshFxpRealpathPacket{}
	case ssh_FXP_STAT:
		pkt = &sshFxpStatPacket{}
	case ssh_FXP_RENAME:
		pkt = &sshFxpRenamePacket{}
	case ssh_FXP_READLINK:
		pkt = &sshFxpReadlinkPacket{}
	case ssh_FXP_SYMLINK:
		pkt = &sshFxpSymlinkPacket{}
	case ssh_FXP_EXTENDED:
		pkt = &sshFxpExtendedPacket{}
	default:
		return nil, errors.Errorf("unhandled packet type: %s", p.pktType)
	}
	if err := pkt.UnmarshalBinary(p.pktBytes); err != nil {
		return nil, err
	}
	return pkt, nil
}
