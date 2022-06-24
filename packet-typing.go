package sftp

import (
	"encoding"
	"fmt"
)

// all incoming packets
type RequestPacket interface {
	encoding.BinaryUnmarshaler
	id() uint32
}

type responsePacket interface {
	encoding.BinaryMarshaler
	id() uint32
}

// interfaces to group types
type hasPath interface {
	RequestPacket
	getPath() string
}

type hasHandle interface {
	RequestPacket
	getHandle() string
}

type notReadOnly interface {
	notReadOnly()
}

//// define types by adding methods
// hasPath
func (p *LstatPacket) getPath() string         { return p.Path }
func (p *StatPacket) getPath() string          { return p.Path }
func (p *RmdirPacket) getPath() string         { return p.Path }
func (p *ReadlinkPacket) getPath() string      { return p.Path }
func (p *RealpathPacket) getPath() string      { return p.Path }
func (p *MkdirPacket) getPath() string         { return p.Path }
func (p *SetstatPacket) getPath() string       { return p.Path }
func (p *sshFxpStatvfsPacket) getPath() string { return p.Path }
func (p *RemovePacket) getPath() string        { return p.Filename }
func (p *RenamePacket) getPath() string        { return p.Oldpath }
func (p *SymlinkPacket) getPath() string       { return p.Targetpath }
func (p *OpendirPacket) getPath() string       { return p.Path }
func (p *OpenPacket) getPath() string          { return p.Path }

func (p *sshFxpExtendedPacketPosixRename) getPath() string { return p.Oldpath }
func (p *sshFxpExtendedPacketHardlink) getPath() string    { return p.Oldpath }

// getHandle
func (p *FstatPacket) getHandle() string    { return p.Handle }
func (p *FsetstatPacket) getHandle() string { return p.Handle }
func (p *ReadPacket) getHandle() string     { return p.Handle }
func (p *WritePacket) getHandle() string    { return p.Handle }
func (p *ReaddirPacket) getHandle() string  { return p.Handle }
func (p *ClosePacket) getHandle() string    { return p.Handle }

// notReadOnly
func (p *WritePacket) notReadOnly()                     {}
func (p *SetstatPacket) notReadOnly()                   {}
func (p *FsetstatPacket) notReadOnly()                  {}
func (p *RemovePacket) notReadOnly()                    {}
func (p *MkdirPacket) notReadOnly()                     {}
func (p *RmdirPacket) notReadOnly()                     {}
func (p *RenamePacket) notReadOnly()                    {}
func (p *SymlinkPacket) notReadOnly()                   {}
func (p *sshFxpExtendedPacketPosixRename) notReadOnly() {}
func (p *sshFxpExtendedPacketHardlink) notReadOnly()    {}

// some packets with ID are missing id()
func (p *sshFxpDataPacket) id() uint32   { return p.ID }
func (p *sshFxpStatusPacket) id() uint32 { return p.ID }
func (p *sshFxpStatResponse) id() uint32 { return p.ID }
func (p *sshFxpNamePacket) id() uint32   { return p.ID }
func (p *sshFxpHandlePacket) id() uint32 { return p.ID }
func (p *StatVFS) id() uint32            { return p.ID }
func (p *sshFxVersionPacket) id() uint32 { return 0 }

// take raw incoming packet data and build packet objects
func makePacket(p rxPacket) (RequestPacket, error) {
	var pkt RequestPacket
	switch p.pktType {
	case sshFxpInit:
		pkt = &sshFxInitPacket{}
	case sshFxpLstat:
		pkt = &LstatPacket{}
	case sshFxpOpen:
		pkt = &OpenPacket{}
	case sshFxpClose:
		pkt = &ClosePacket{}
	case sshFxpRead:
		pkt = &ReadPacket{}
	case sshFxpWrite:
		pkt = &WritePacket{}
	case sshFxpFstat:
		pkt = &FstatPacket{}
	case sshFxpSetstat:
		pkt = &SetstatPacket{}
	case sshFxpFsetstat:
		pkt = &FsetstatPacket{}
	case sshFxpOpendir:
		pkt = &OpendirPacket{}
	case sshFxpReaddir:
		pkt = &ReaddirPacket{}
	case sshFxpRemove:
		pkt = &RemovePacket{}
	case sshFxpMkdir:
		pkt = &MkdirPacket{}
	case sshFxpRmdir:
		pkt = &RmdirPacket{}
	case sshFxpRealpath:
		pkt = &RealpathPacket{}
	case sshFxpStat:
		pkt = &StatPacket{}
	case sshFxpRename:
		pkt = &RenamePacket{}
	case sshFxpReadlink:
		pkt = &ReadlinkPacket{}
	case sshFxpSymlink:
		pkt = &SymlinkPacket{}
	case sshFxpExtended:
		pkt = &sshFxpExtendedPacket{}
	default:
		return nil, fmt.Errorf("unhandled packet type: %s", p.pktType)
	}
	if err := pkt.UnmarshalBinary(p.pktBytes); err != nil {
		// Return partially unpacked packet to allow callers to return
		// error messages appropriately with necessary id() method.
		return pkt, err
	}
	return pkt, nil
}
