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
	ExtendedRequest string

	Data ExtendedData
}

// MarshalPacket returns p as a two-part binary encoding of p.
//
// The Data is marshaled into binary, and returned as the payload.
func (p *ExtendedPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.ExtendedRequest) // string(extended-request)

	b := NewMarshalBuffer(PacketTypeExtended, reqid, size)

	b.AppendString(p.ExtendedRequest)

	if p.Data != nil {
		payload, err = p.Data.MarshalBinary()
		if err != nil {
			return nil, nil, err
		}
	}

	return b.Packet(payload)
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

// ExtendedReplyPacket defines the SSH_FXP_CLOSE packet.
type ExtendedReplyPacket struct {
	Data ExtendedData
}

// MarshalPacket returns p as a two-part binary encoding of p.
//
// The Data is marshaled into binary, and returned as the payload.
func (p *ExtendedReplyPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	b := NewMarshalBuffer(PacketTypeExtendedReply, reqid, 0)

	if p.Data != nil {
		payload, err = p.Data.MarshalBinary()
		if err != nil {
			return nil, nil, err
		}
	}

	return b.Packet(payload)
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
