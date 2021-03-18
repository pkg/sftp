package filexfer

// ClosePacket defines the SSH_FXP_CLOSE packet.
type ClosePacket struct {
	RequestID uint32
	Handle    string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ClosePacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Handle) // string

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeClose))
	b.AppendUint32(p.RequestID)
	b.AppendString(p.Handle)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *ClosePacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ClosePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *ClosePacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// ReadPacket defines the SSH_FXP_READ packet.
type ReadPacket struct {
	RequestID uint32
	Handle    string
	Offset    uint64
	Len       uint32
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ReadPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Handle) + 8 + 4 // string + uint64(offset) + uint32(len)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeRead))
	b.AppendUint32(p.RequestID)
	b.AppendString(p.Handle)
	b.AppendUint64(p.Offset)
	b.AppendUint32(p.Len)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *ReadPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ReadPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	if p.Offset, err = buf.ConsumeUint64(); err != nil {
		return err
	}

	if p.Len, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *ReadPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// WritePacket defines the SSH_FXP_WRITE packet.
type WritePacket struct {
	RequestID uint32
	Handle    string
	Offset    uint64
	Data      []byte
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *WritePacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Handle) + 8 + 4 // string + uint64(offset) + uint32(len(data))

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeWrite))
	b.AppendUint32(p.RequestID)
	b.AppendString(p.Handle)
	b.AppendUint64(p.Offset)
	b.AppendUint32(uint32(len(p.Data)))

	b.PutLength(size + len(p.Data))

	return b.Bytes(), p.Data, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *WritePacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *WritePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	if p.Offset, err = buf.ConsumeUint64(); err != nil {
		return err
	}

	if p.Data, err = buf.ConsumeByteSlice(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *WritePacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// FstatPacket defines the SSH_FXP_FSTAT packet.
type FstatPacket struct {
	RequestID uint32
	Handle    string
	Flags     uint32
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *FstatPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Handle) + 4 // string + uint32(flags)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeFstat))
	b.AppendUint32(p.RequestID)
	b.AppendString(p.Handle)
	b.AppendUint32(p.Flags)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *FstatPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *FstatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	if p.Flags, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *FstatPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// FsetstatPacket defines the SSH_FXP_FSETSTAT packet.
type FsetstatPacket struct {
	RequestID uint32
	Handle    string
	Attrs     Attributes
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *FsetstatPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Handle) // string

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeFsetstat))
	b.AppendUint32(p.RequestID)
	b.AppendString(p.Handle)

	p.Attrs.MarshalInto(b)

	b.PutLength(b.Len() - 4)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *FsetstatPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *FsetstatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *FsetstatPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// ReaddirPacket defines the SSH_FXP_READDIR packet.
type ReaddirPacket struct {
	RequestID uint32
	Handle    string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ReaddirPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Handle) // string

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeReaddir))
	b.AppendUint32(p.RequestID)
	b.AppendString(p.Handle)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *ReaddirPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ReaddirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *ReaddirPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}
