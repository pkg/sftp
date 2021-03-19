package filexfer

import (
	"encoding"
)

// ExtendedData aliases the untyped interface composition of encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type ExtendedData = interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// ExtendedPacket defines the SSH_FXP_CLOSE packet.
type ExtendedPacket struct {
	RequestID       uint32
	ExtendedRequest string

	Data ExtendedData
}

// MarshalPacket returns p as a two-part binary encoding of p.
//
// The Data is marshaled into binary, and returned as the payload.
func (p *ExtendedPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.ExtendedRequest) // string(extended-request)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeExtended))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.ExtendedRequest)

	if p.Data != nil {
		payload, err = p.Data.MarshalBinary()
		if err != nil {
			return nil, nil, err
		}
	}

	b.PutLength(size + len(payload))

	return b.Bytes(), payload, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *ExtendedPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
//
// If p.Data is nil, and there is request-specific-data,
// then the request-specific-data will be wrapped in a Buffer and assigned to p.Data.
func (p *ExtendedPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.ExtendedRequest, err = buf.ConsumeString(); err != nil {
		return err
	}

	if buf.Len() > 0 {
		if p.Data == nil {
			p.Data = new(Buffer)
		}

		if err := p.Data.UnmarshalBinary(buf.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *ExtendedPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// ExtendedReplyPacket defines the SSH_FXP_CLOSE packet.
type ExtendedReplyPacket struct {
	RequestID uint32

	Data ExtendedData
}

// MarshalPacket returns p as a two-part binary encoding of p.
//
// The Data is marshaled into binary, and returned as the payload.
func (p *ExtendedReplyPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 // byte(type) + uint32(request-id)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeExtendedReply))
	b.AppendUint32(p.RequestID)

	if p.Data != nil {
		payload, err = p.Data.MarshalBinary()
		if err != nil {
			return nil, nil, err
		}
	}

	b.PutLength(size + len(payload))

	return b.Bytes(), payload, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *ExtendedReplyPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
//
// If p.Data is nil, and there is request-specific-data,
// then the request-specific-data will be wrapped in a Buffer and assigned to p.Data.
func (p *ExtendedReplyPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if buf.Len() > 0 {
		if p.Data == nil {
			p.Data = new(Buffer)
		}

		if err := p.Data.UnmarshalBinary(buf.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *ExtendedReplyPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}
