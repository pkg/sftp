package sshfx

import (
	"encoding"
	"reflect"
	"sync"
)

// ExtendedData aliases the untyped interface composition of encoding.BinaryMarshaler and encoding.BinaryUnmarshaler.
type ExtendedData = interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// ExtendedRequestPacket defines the interface an extended request packet should implement.
type ExtendedRequestPacket interface{
	ExtendedData

	// Type ensures it is packet-like, it should always return PacketTypeExtended.
	Type() PacketType

	// ExtendedRequest is the extended-request name that the packet defines.
	ExtendedRequest() string
}

// extendedDataConstructor defines a function that returns a new(ArbitraryExtendedPacket).
type extendedDataConstructor struct{
	typ reflect.Type
	new func() ExtendedData
}

var extendedPacketTypes struct {
	mu           sync.RWMutex
	constructors map[string]*extendedDataConstructor
}

// RegisterExtendedPacketType defines a specific ExtendedDataConstructor for the given extended request name.
//
// This operation is idempotent so long as the ExtendedRequest name is only being registered with the same type.
func RegisterExtendedPacketType[PKT any, EXT interface{ ExtendedRequestPacket ; *PKT }]() {
	extendedPacketTypes.mu.Lock()
	defer extendedPacketTypes.mu.Unlock()

	var prototype EXT
	extension := prototype.ExtendedRequest()

	typ := reflect.TypeFor[PKT]()

	if has, exist := extendedPacketTypes.constructors[extension]; exist {
		if has.typ == typ {
			return
		}

		panic("encoding/ssh/filexfer: conflicting registrations of extended packet type " + extension)
	}

	if extendedPacketTypes.constructors == nil {
		extendedPacketTypes.constructors = make(map[string]*extendedDataConstructor)
	}

	extendedPacketTypes.constructors[extension] = &extendedDataConstructor{
		typ: typ,
		new: func() ExtendedData {
			return EXT(new(PKT))
		},
	}
}

func newExtendedPacket(extension string) ExtendedData {
	extendedPacketTypes.mu.RLock()
	defer extendedPacketTypes.mu.RUnlock()

	if con := extendedPacketTypes.constructors[extension]; con != nil {
		return con.new()
	}

	return new(Buffer)
}

// ExtendedPacket defines the SSH_FXP_CLOSE packet.
type ExtendedPacket struct {
	ExtendedRequest string

	Data ExtendedData
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *ExtendedPacket) Type() PacketType {
	return PacketTypeExtended
}

// MarshalPacket returns p as a two-part binary encoding of p.
//
// The Data is marshaled into binary, and returned as the payload.
func (p *ExtendedPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		size := 4 + len(p.ExtendedRequest) // string(extended-request)
		buf = NewMarshalBuffer(size)
	}

	buf.StartPacket(PacketTypeExtended, reqid)
	buf.AppendString(p.ExtendedRequest)

	if p.Data != nil {
		payload, err = p.Data.MarshalBinary()
		if err != nil {
			return nil, nil, err
		}
	}

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
//
// If p.Data is nil, and the extension has been registered, a new type will be made from the registration.
// If the extension has not been registered, then a new Buffer will be allocated.
// Then the request-specific-data will be unmarshaled from the rest of the buffer.
func (p *ExtendedPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	p.ExtendedRequest = buf.ConsumeString()
	if buf.Err != nil {
		return buf.Err
	}

	if p.Data == nil {
		p.Data = newExtendedPacket(p.ExtendedRequest)
	}

	return p.Data.UnmarshalBinary(buf.Bytes())
}

// ExtendedReplyPacket defines the SSH_FXP_CLOSE packet.
type ExtendedReplyPacket struct {
	Data ExtendedData
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *ExtendedReplyPacket) Type() PacketType {
	return PacketTypeExtendedReply
}

// MarshalPacket returns p as a two-part binary encoding of p.
//
// The Data is marshaled into binary, and returned as the payload.
func (p *ExtendedReplyPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(0)
	}

	buf.StartPacket(PacketTypeExtendedReply, reqid)

	if p.Data != nil {
		payload, err = p.Data.MarshalBinary()
		if err != nil {
			return nil, nil, err
		}
	}

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
//
// If p.Data is nil, and there is request-specific-data,
// then the request-specific-data will be wrapped in a Buffer and assigned to p.Data.
func (p *ExtendedReplyPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Data == nil {
		p.Data = new(Buffer)
	}

	return p.Data.UnmarshalBinary(buf.Bytes())
}
