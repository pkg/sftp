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
// RawPacket is intended for use in clients receiving responses,
// where a response will be expected to be of a limited number of types,
// and unmarshaling unknown/unexpected response packets is unnecessary.
//
// For servers expecting to receive arbitrary request packet types,
// use RequestPacket.
//
// Defined in https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-3
type RawPacket struct {
	Type      PacketType
	RequestID uint32

	Data Buffer
}

// Reset clears the pointers and reference-semantic variables of RawPacket,
// releasing underlying resources, and making them and the RawPacket suitable to be reused,
// so long as no other references have been kept.
func (p *RawPacket) Reset() {
	p.Data = Buffer{}
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
//
// This is a convenience implementation primarily intended for tests,
// because it is inefficient with allocations.
func (p *RawPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket(p.RequestID, nil))
}

// UnmarshalFrom decodes a RawPacket from the given Buffer into p.
//
// The Data field will alias the passed in Buffer,
// so the buffer passed in should not be reused before RawPacket.Reset().
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
// This is a convenience implementation primarily intended for tests,
// because this must clone the given data byte slice,
// as Data is not allowed to alias any part of the data byte slice.
func (p *RawPacket) UnmarshalBinary(data []byte) error {
	clone := make([]byte, len(data))
	n := copy(clone, data)
	return p.UnmarshalFrom(NewBuffer(clone[:n]))
}

// readPacket reads a uint32 length-prefixed binary data packet from r.
// using the given byte slice as a backing array.
//
// If the packet length read from r is bigger than maxPacketLength,
// or greater than math.MaxInt32 on a 32-bit implementation,
// then a `ErrLongPacket` error will be returned.
//
// If the given byte slice is insufficient to hold the packet,
// then it will be extended to fill the packet size.
func readPacket(r io.Reader, b []byte, maxPacketLength uint32) ([]byte, error) {
	if len(b) < 64 {
		b = make([]byte, 64)
	}

	if _, err := io.ReadFull(r, b[:4]); err != nil {
		return nil, err
	}

	length := unmarshalUint32(b)
	if int(length) < 5 {
		if int(length) < 0 {
			// Only possible when strconv.IntSize == 32,
			// the packet length is longer than math.MaxInt32,
			// and thus longer than any possible slice.
			return nil, ErrLongPacket
		}

		// Must have at least uint8(type) and uint32(request-id)
		return nil, ErrShortPacket
	}
	if length > maxPacketLength {
		return nil, ErrLongPacket
	}

	if int(length) > cap(b) {
		// We know int(length) must be positive, because of tests above.
		b = make([]byte, length)
	}

	n, err := io.ReadFull(r, b[:length])
	return b[:n], err
}

// ReadFrom provides a simple functional packet reader,
// using the given byte slice as a backing array.
//
// To protect against potential denial of service attacks,
// if the read packet length is longer than maxPacketLength,
// then no packet data will be read, and ErrLongPacket will be returned.
// (On 32-bit int architectures, all packets >= 2^31 in length
// will return ErrLongPacket regardless of maxPacketLength.)
//
// If the read packet length is longer than cap(b),
// then a throw-away slice will allocated to meet the exact packet length.
// This can be used to limit the length of reused buffers,
// while still allowing reception of occassional large packets.
//
// The Data field may alias the passed in byte slice,
// so the byte slice passed in should not be reused before RawPacket.Reset().
func (p *RawPacket) ReadFrom(r io.Reader, b []byte, maxPacketLength uint32) error {
	b, err := readPacket(r, b, maxPacketLength)
	if err != nil {
		return err
	}

	return p.UnmarshalFrom(NewBuffer(b))
}

// RequestPacket implements the general packet format from draft-ietf-secsh-filexfer-02
// but also automatically decode/encodes valid request packets (2 < type < 100 || type == 200).
//
// RequestPacket is intended for use in servers receiving requests,
// where any arbitrary request may be received, and so decoding them automatically
// is useful.
//
// For clients expecting to receive specific response packet types,
// where automatic unmarshaling of the packet body does not make sense,
// use RawPacket.
//
// Defined in https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-3
type RequestPacket struct {
	RequestID uint32

	Request Packet
}

// Reset clears the pointers and reference-semantic variables in RequestPacket,
// releasing underlying resources, and making them and the RequestPacket suitable to be reused,
// so long as no other references have been kept.
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
//
// This is a convenience implementation primarily intended for tests,
// because it is inefficient with allocations.
func (p *RequestPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket(p.RequestID, nil))
}

// UnmarshalFrom decodes a RequestPacket from the given Buffer into p.
//
// The Request field may alias the passed in Buffer, (e.g. SSH_FXP_WRITE),
// so the buffer passed in should not be reused before RequestPacket.Reset().
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

// UnmarshalBinary decodes a full request packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
//
// This is a convenience implementation primarily intended for tests,
// because this must clone the given data byte slice,
// as Request is not allowed to alias any part of the data byte slice.
func (p *RequestPacket) UnmarshalBinary(data []byte) error {
	clone := make([]byte, len(data))
	n := copy(clone, data)
	return p.UnmarshalFrom(NewBuffer(clone[:n]))
}

// ReadFrom provides a simple functional packet reader,
// using the given byte slice as a backing array.
//
// To protect against potential denial of service attacks,
// if the read packet length is longer than maxPacketLength,
// then no packet data will be read, and ErrLongPacket will be returned.
// (On 32-bit int architectures, all packets >= 2^31 in length
// will return ErrLongPacket regardless of maxPacketLength.)
//
// If the read packet length is longer than cap(b),
// then a throw-away slice will allocated to meet the exact packet length.
// This can be used to limit the length of reused buffers,
// while still allowing reception of occassional large packets.
//
// The Request field may alias the passed in byte slice,
// so the byte slice passed in should not be reused before RawPacket.Reset().
func (p *RequestPacket) ReadFrom(r io.Reader, b []byte, maxPacketLength uint32) error {
	b, err := readPacket(r, b, maxPacketLength)
	if err != nil {
		return err
	}

	return p.UnmarshalFrom(NewBuffer(b))
}
