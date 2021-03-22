package filexfer

// ClosePacket defines the SSH_FXP_CLOSE packet.
type ClosePacket struct {
	Handle string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ClosePacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Handle) // string(handle)

	b := NewMarshalBuffer(PacketTypeClose, reqid, size)

	b.AppendString(p.Handle)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ClosePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// ReadPacket defines the SSH_FXP_READ packet.
type ReadPacket struct {
	Handle string
	Offset uint64
	Len    uint32
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ReadPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	// string(handle) + uint64(offset) + uint32(len)
	size := 4 + len(p.Handle) + 8 + 4

	b := NewMarshalBuffer(PacketTypeRead, reqid, size)

	b.AppendString(p.Handle)
	b.AppendUint64(p.Offset)
	b.AppendUint32(p.Len)

	return b.Packet(payload)
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

// WritePacket defines the SSH_FXP_WRITE packet.
type WritePacket struct {
	Handle string
	Offset uint64
	Data   []byte
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *WritePacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	// string(handle) + uint64(offset) + uint32(len(data)); data content in payload
	size := 4 + len(p.Handle) + 8 + 4

	b := NewMarshalBuffer(PacketTypeWrite, reqid, size)

	b.AppendString(p.Handle)
	b.AppendUint64(p.Offset)
	b.AppendUint32(uint32(len(p.Data)))

	return b.Packet(p.Data)
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

// FStatPacket defines the SSH_FXP_FSTAT packet.
type FStatPacket struct {
	Handle string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *FStatPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Handle) // string(handle)

	b := NewMarshalBuffer(PacketTypeFStat, reqid, size)

	b.AppendString(p.Handle)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *FStatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// FSetstatPacket defines the SSH_FXP_FSETSTAT packet.
type FSetstatPacket struct {
	Handle string
	Attrs  Attributes
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *FSetstatPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Handle) + p.Attrs.Len() // string(handle) + ATTRS(attrs)

	b := NewMarshalBuffer(PacketTypeFSetstat, reqid, size)

	b.AppendString(p.Handle)

	p.Attrs.MarshalInto(b)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *FSetstatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// ReadDirPacket defines the SSH_FXP_READDIR packet.
type ReadDirPacket struct {
	Handle string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ReadDirPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Handle) // string(handle)

	b := NewMarshalBuffer(PacketTypeReadDir, reqid, size)

	b.AppendString(p.Handle)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ReadDirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Handle, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}
