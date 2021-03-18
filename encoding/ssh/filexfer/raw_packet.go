package filexfer

// RawPacket implements the general packet format from draft-ietf-secsh-filexfer-02
//
// Defined in https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-3
type RawPacket struct {
	Type      PacketType
	RequestID uint32
	Payload   []byte
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RawPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 // byte(type) + uint32(id)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(p.Type))
	b.AppendUint32(p.RequestID)

	b.PutLength(size + len(p.Payload))

	return b.Bytes(), p.Payload, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *RawPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
//
// NOTE: To avoid extra allocations, UnmarshalPackBody aliases the underlying Buffer’s byte slice.
func (p *RawPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	p.Payload = buf.Bytes()
	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
//
// NOTE: To avoid extra allocations, UnmarshalBinary aliases the given byte slice.
func (p *RawPacket) UnmarshalBinary(data []byte) error {
	buf := NewBuffer(data)

	typ, err := buf.ConsumeUint8()
	if err != nil {
		return err
	}

	p.Type = PacketType(typ)

	return p.UnmarshalPacketBody(buf)
}
