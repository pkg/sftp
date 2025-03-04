package sshfx

import (
	"fmt"
	"io"
)

// InitPacket defines the SSH_FXP_INIT packet.
type InitPacket struct {
	Version    uint32
	Extensions []*ExtensionPair
}

// MarshalBinary returns p as the binary encoding of p.
func (p *InitPacket) MarshalBinary() ([]byte, error) {
	size := 1 + 4 // byte(type) + uint32(version)

	for _, ext := range p.Extensions {
		size += ext.MarshalSize()
	}

	b := NewBuffer(make([]byte, 4, 4+size))
	b.AppendUint8(uint8(PacketTypeInit))
	b.AppendUint32(p.Version)

	for _, ext := range p.Extensions {
		ext.MarshalInto(b)
	}

	b.PutLength(size)

	return b.Bytes(), nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *InitPacket) UnmarshalBinary(data []byte) error {
	return p.UnmarshalFrom(NewBuffer(data))
}

// UnmarshalFrom decodes a RawPacket from the given Buffer into p.
//
// The Data field will alias the passed in Buffer,
// so the buffer passed in should not be reused before RawPacket.Reset().
func (p *InitPacket) UnmarshalFrom(buf *Buffer) error {
	*p = InitPacket{
		Version: buf.ConsumeUint32(),
	}

	for buf.Len() > 0 {
		var ext ExtensionPair
		if err := ext.UnmarshalFrom(buf); err != nil {
			return err
		}

		p.Extensions = append(p.Extensions, &ext)
	}

	return buf.Err
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
// while still allowing reception of occasional large packets.
func (p *InitPacket) ReadFrom(r io.Reader, b []byte, maxPacketLength uint32) error {
	b, err := readPacket(r, b, maxPacketLength)
	if err != nil {
		return err
	}

	buf := NewBuffer(b)

	typ := PacketType(buf.ConsumeUint8())
	if buf.Err != nil {
		return buf.Err
	}

	if typ != PacketTypeInit {
		return fmt.Errorf("sshfx: invalid init packet: wrong type: %s", typ)
	}

	return p.UnmarshalFrom(buf)
}

// VersionPacket defines the SSH_FXP_VERSION packet.
type VersionPacket struct {
	Version    uint32
	Extensions []*ExtensionPair
}

// MarshalBinary returns p as the binary encoding of p.
func (p *VersionPacket) MarshalBinary() ([]byte, error) {
	size := 1 + 4 // byte(type) + uint32(version)

	for _, ext := range p.Extensions {
		size += ext.MarshalSize()
	}

	b := NewBuffer(make([]byte, 4, 4+size))
	b.AppendUint8(uint8(PacketTypeVersion))
	b.AppendUint32(p.Version)

	for _, ext := range p.Extensions {
		ext.MarshalInto(b)
	}

	b.PutLength(size)

	return b.Bytes(), nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *VersionPacket) UnmarshalBinary(data []byte) (err error) {
	return p.UnmarshalFrom(NewBuffer(data))
}

// UnmarshalFrom decodes a RawPacket from the given Buffer into p.
//
// The Data field will alias the passed in Buffer,
// so the buffer passed in should not be reused before RawPacket.Reset().
func (p *VersionPacket) UnmarshalFrom(buf *Buffer) error {
	*p = VersionPacket{
		Version: buf.ConsumeUint32(),
	}

	for buf.Len() > 0 {
		var ext ExtensionPair
		if err := ext.UnmarshalFrom(buf); err != nil {
			return err
		}

		p.Extensions = append(p.Extensions, &ext)
	}

	return nil
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
// while still allowing reception of occasional large packets.
func (p *VersionPacket) ReadFrom(r io.Reader, b []byte, maxPacketLength uint32) error {
	b, err := readPacket(r, b, maxPacketLength)
	if err != nil {
		return err
	}

	buf := NewBuffer(b)

	typ := PacketType(buf.ConsumeUint8())
	if buf.Err != nil {
		return buf.Err
	}

	if typ != PacketTypeVersion {
		return fmt.Errorf("sshfx: invalid version packet: wrong type: %s", typ)
	}

	return p.UnmarshalFrom(buf)
}
