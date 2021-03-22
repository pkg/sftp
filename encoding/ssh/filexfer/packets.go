package filexfer

import (
	"errors"
	"fmt"
	"io"
)

func newPacketFromType(typ PacketType) (Packet, error) {
	switch typ {
	case PacketTypeOpen:
		return new(OpenPacket), nil
	case PacketTypeClose:
		return new(ClosePacket), nil
	case PacketTypeRead:
		return new(ReadPacket), nil
	case PacketTypeWrite:
		return new(WritePacket), nil
	case PacketTypeLStat:
		return new(LStatPacket), nil
	case PacketTypeFStat:
		return new(FStatPacket), nil
	case PacketTypeSetstat:
		return new(SetstatPacket), nil
	case PacketTypeFSetstat:
		return new(FSetstatPacket), nil
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
		return nil, fmt.Errorf("unexpected request packet type: %v", typ)
	}
}

// RawPacket implements the general packet format from draft-ietf-secsh-filexfer-02
//
// Defined in https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-3
type RawPacket struct {
	Type      PacketType
	RequestID uint32

	Data Buffer
}

// MarshalPacket returns p as a two-part binary encoding of p.
//
// The internal p.RequestID is overridden by the reqid argument.
func (p *RawPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(0)
	}

	buf.StartPacket(p.Type, reqid)

	return buf.Packet(p.Data.Bytes())
}

// MarshalBinary returns p as the binary encoding of p.
func (p *RawPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket(p.RequestID, nil))
}

// UnmarshalFrom decodes a RawPacket from the given Buffer into p.
//
// The Data field will take ownership of the underyling byte slice of buf.
// The caller should not use buf after this call.
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

// defaultMaxPacketSize is defined in draft-ietf-secsh-filexfer-02 section 3.
const defaultMaxPacketSize = 34000

// readPacket reads a uint32 length-prefixed binary data packet from r.
// If the given buffer is less than 4-bytes, it allocates a new buffer of size defaultMaxPacketSize.
func readPacket(r io.Reader, b []byte) ([]byte, error) {
	if len(b) < 4 {
		b = make([]byte, defaultMaxPacketSize)
	}

	if _, err := io.ReadFull(r, b[:4]); err != nil {
		return nil, err
	}

	length := unmarshalUint32(b)
	if length < 1 {
		return nil, ErrShortPacket
	}
	if length > uint32(len(b)) {
		return nil, ErrLongPacket
	}

	n, err := io.ReadFull(r, b[:length])
	return b[:n], err
}

// ReadFrom reads a full raw packet out of the given reader.
func (p *RawPacket) ReadFrom(r io.Reader, b []byte) error {
	b, err := readPacket(r, b)
	if err != nil {
		return err
	}

	return p.UnmarshalBinary(b)
}

// RequestPacket decodes a full RequestPacket from the internal Data based on the Type.
// Prefer using a RequestPacket directly, rather than going indirectly through RawPacket.
func (p *RawPacket) RequestPacket() (*RequestPacket, error) {
	packet, err := newPacketFromType(p.Type)
	if err != nil {
		return nil, err
	}

	packet.UnmarshalPacketBody(&p.Data)

	return &RequestPacket{
		RequestID: p.RequestID,

		Request: packet,
	}, nil
}

// RequestPacket implements the general packet format from draft-ietf-secsh-filexfer-02
//
// Defined in https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-3
type RequestPacket struct {
	RequestID uint32

	Request Packet
}

// Reset clears the pointers and reference-semantic variables of RequestPacket,
// making it suitable to be put into a sync.Pool.
func (p *RequestPacket) Reset() {
	p.Request = nil
}

// MarshalPacket returns p as a two-part binary encoding of p.
//
// The internal p.RequestID is overridden by the reqid argument.
func (p *RequestPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	if p.Request == nil {
		return nil, nil, errors.New("empty request packet")
	}

	return p.Request.MarshalPacket(reqid, b)
}

// MarshalBinary returns p as the binary encoding of p.
func (p *RequestPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket(p.RequestID, nil))
}

// UnmarshalFrom decodes a RequestPacket from the given Buffer into p.
func (p *RequestPacket) UnmarshalFrom(buf *Buffer) error {
	typ, err := buf.ConsumeUint8()
	if err != nil {
		return err
	}

	p.Request, err = newPacketFromType(PacketType(typ))
	if err != nil {
		return err
	}

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.Request.UnmarshalPacketBody(buf)
}

// UnmarshalBinary decodes a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
//
// NOTE: To avoid extra allocations, UnmarshalBinary aliases the given byte slice.
func (p *RequestPacket) UnmarshalBinary(data []byte) error {
	return p.UnmarshalFrom(NewBuffer(data))
}

// ReadFrom reads a full raw packet out of the given reader.
func (p *RequestPacket) ReadFrom(r io.Reader, b []byte) error {
	b, err := readPacket(r, b)
	if err != nil {
		return err
	}

	return p.UnmarshalBinary(b)
}
