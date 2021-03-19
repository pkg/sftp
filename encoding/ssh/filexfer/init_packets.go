package filexfer

// InitPacket defines the SSH_FXP_INIT packet.
type InitPacket struct {
	Version    uint32
	Extensions []*ExtensionPair
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *InitPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 // byte(type) + uint32(version)

	for _, ext := range p.Extensions {
		size += ext.Len()
	}

	b := NewBuffer(make([]byte, 4, 4+size))
	b.AppendUint8(uint8(PacketTypeInit))
	b.AppendUint32(p.Version)

	for _, ext := range p.Extensions {
		ext.MarshalInto(b)
	}

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *InitPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
func (p *InitPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Version, err = buf.ConsumeUint32(); err != nil {
		return err
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

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *InitPacket) UnmarshalBinary(data []byte) error {
	return p.UnmarshalPacketBody(NewBuffer(data))
}

// VersionPacket defines the SSH_FXP_VERSION packet.
type VersionPacket struct {
	Version    uint32
	Extensions []*ExtensionPair
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *VersionPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 // byte(type) + uint32(version)

	for _, ext := range p.Extensions {
		size += ext.Len()
	}

	b := NewBuffer(make([]byte, 4, 4+size))
	b.AppendUint8(uint8(PacketTypeInit))
	b.AppendUint32(p.Version)

	for _, ext := range p.Extensions {
		ext.MarshalInto(b)
	}

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *VersionPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
func (p *VersionPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Version, err = buf.ConsumeUint32(); err != nil {
		return err
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

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *VersionPacket) UnmarshalBinary(data []byte) error {
	return p.UnmarshalPacketBody(NewBuffer(data))
}
