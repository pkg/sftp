package filexfer

// StatusPacket defines the SSH_FXP_STATUS packet.
//
// Specified in https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-7
type StatusPacket struct {
	RequestID    uint32
	StatusCode   Status
	ErrorMessage string
	LanguageTag  string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *StatusPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + // uint32(error/status code)
		4 + len(p.ErrorMessage) + // string(path)
		4 + len(p.LanguageTag) // string(path)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeStatus))
	b.AppendUint32(p.RequestID)

	b.AppendUint32(uint32(p.StatusCode))
	b.AppendString(p.ErrorMessage)
	b.AppendString(p.LanguageTag)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *StatusPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *StatusPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	statusCode, err := buf.ConsumeUint32()
	if err != nil {
		return err
	}
	p.StatusCode = Status(statusCode)

	if p.ErrorMessage, err = buf.ConsumeString(); err != nil {
		return err
	}

	if p.LanguageTag, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *StatusPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// HandlePacket defines the SSH_FXP_HANDLE packet.
type HandlePacket struct {
	RequestID uint32
	Handle    string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *HandlePacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Handle) // string

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeHandle))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.Handle)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *HandlePacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *HandlePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *HandlePacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// DataPacket defines the SSH_FXP_DATA packet.
type DataPacket struct {
	RequestID uint32
	Data      []byte
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *DataPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Data) // string

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeData))
	b.AppendUint32(p.RequestID)

	b.AppendByteSlice(p.Data)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *DataPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *DataPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Data, err = buf.ConsumeByteSlice(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *DataPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// NamePacket defines the SSH_FXP_NAME packet.
type NamePacket struct {
	RequestID uint32
	Entries   []*NameEntry
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *NamePacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 // uint32(count)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeName))
	b.AppendUint32(p.RequestID)

	b.AppendUint32(uint32(len(p.Entries)))

	for _, e := range p.Entries {
		e.MarshalInto(b)
	}

	b.PutLength(b.Len() - 4)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *NamePacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *NamePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	count, err := buf.ConsumeUint32()
	if err != nil {
		return err
	}

	p.Entries = make([]*NameEntry, 0, count)

	for i := uint32(0); i < count; i++ {
		var e NameEntry
		if err := e.UnmarshalFrom(buf); err != nil {
			return err
		}

		p.Entries = append(p.Entries, &e)
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *NamePacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// AttrsPacket defines the SSH_FXP_ATTRS packet.
type AttrsPacket struct {
	RequestID uint32
	Attrs     Attributes
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *AttrsPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 // byte(type) + uint32(request-id)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeAttrs))
	b.AppendUint32(p.RequestID)

	p.Attrs.MarshalInto(b)

	b.PutLength(b.Len() - 4)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *AttrsPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *AttrsPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	return p.Attrs.UnmarshalFrom(buf)
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *AttrsPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}
