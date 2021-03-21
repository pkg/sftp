package filexfer

import (
	"fmt"
	"io"
)

// RawPacket implements the general packet format from draft-ietf-secsh-filexfer-02
//
// Defined in https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-3
type RawPacket struct {
	Type      PacketType
	RequestID uint32

	Data Buffer
}

// Reset clears the pointers and reference-semantic variables of RawPacket,
// making it suitable to be put into a sync.Pool.
func (p *RawPacket) Reset() {
	p.Data.Reset()
}

// MarshalPacket returns p as a two-part binary encoding of p.
//
// The internal p.RequestID is overridden by the reqid argument.
func (p *RawPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	b := NewMarshalBuffer(p.Type, reqid, 0)

	return b.Packet(p.Data.Bytes())
}

// MarshalBinary returns p as the binary encoding of p.
func (p *RawPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket(p.RequestID))
}

// UnmarshalFrom decodes a RawPacket from the given Buffer into p.
func (p *RawPacket) UnmarshalFrom(buf *Buffer) error {
	typ, err := buf.ConsumeUint8()
	if err != nil {
		return err
	}

	p.Type = PacketType(typ)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	p.Data = *buf
	return nil
}

// UnmarshalBinary decodes a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
//
// NOTE: To avoid extra allocations, UnmarshalBinary aliases the given byte slice.
func (p *RawPacket) UnmarshalBinary(data []byte) error {
	return p.UnmarshalFrom(NewBuffer(data))
}

// ReadFrom reads a full raw packet out of the given reader.
func (p *RawPacket) ReadFrom(r io.Reader, maxPacketLength uint32) error {
	lb := make([]byte, 4)
	if _, err := io.ReadFull(r, lb); err != nil {
		return err
	}

	length := unmarshalUint32(lb)
	if length < 1 {
		return ErrShortPacket
	}
	if length > maxPacketLength {
		return ErrLongPacket
	}

	b := make([]byte, length)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}

	return p.UnmarshalBinary(b)
}

// RequestPacket decodes a full request packet from the internal Data based on the Type.
func (p *RawPacket) RequestPacket() (reqid uint32, packet Packet, err error) {
	switch p.Type {
	case PacketTypeOpen:
		packet = new(OpenPacket)
	case PacketTypeClose:
		packet = new(ClosePacket)
	case PacketTypeRead:
		packet = new(ReadPacket)
	case PacketTypeWrite:
		packet = new(WritePacket)
	case PacketTypeLstat:
		packet = new(LstatPacket)
	case PacketTypeFstat:
		packet = new(FstatPacket)
	case PacketTypeSetstat:
		packet = new(SetstatPacket)
	case PacketTypeFsetstat:
		packet = new(FsetstatPacket)
	case PacketTypeOpendir:
		packet = new(OpendirPacket)
	case PacketTypeReaddir:
		packet = new(ReaddirPacket)
	case PacketTypeRemove:
		packet = new(RemovePacket)
	case PacketTypeMkdir:
		packet = new(MkdirPacket)
	case PacketTypeRmdir:
		packet = new(RmdirPacket)
	case PacketTypeRealpath:
		packet = new(RealpathPacket)
	case PacketTypeStat:
		packet = new(StatPacket)
	case PacketTypeRename:
		packet = new(RenamePacket)
	case PacketTypeReadlink:
		packet = new(ReadlinkPacket)
	case PacketTypeSymlink:
		packet = new(SymlinkPacket)
	case PacketTypeExtended:
		packet = new(ExtendedPacket)
	default:
		return p.RequestID, nil, fmt.Errorf("unexpected packet request type: %v", p.Type)
	}

	packet.UnmarshalPacketBody(&p.Data)
	return p.RequestID, packet, err
}
