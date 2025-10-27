package sshfx

// ClosePacket defines the SSH_FXP_CLOSE packet.
type ClosePacket struct {
	Handle string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *ClosePacket) Type() PacketType {
	return PacketTypeClose
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *ClosePacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(handle)
	return 1 + 4 + 4 + len(p.Handle)
}

// GetHandle returns the handle field of the packet.
func (p *ClosePacket) GetHandle() string {
	return p.Handle
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ClosePacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeClose, reqid)
	buf.AppendString(p.Handle)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ClosePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = ClosePacket{
		Handle: buf.ConsumeString(),
	}

	return buf.Err
}

// ReadPacket defines the SSH_FXP_READ packet.
type ReadPacket struct {
	Handle string
	Offset uint64
	Length uint32
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *ReadPacket) Type() PacketType {
	return PacketTypeRead
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *ReadPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(handle) + uint64(offset) + uint32(len)
	return 1 + 4 + 4 + len(p.Handle) + 8 + 4
}

// GetHandle returns the handle field of the packet.
func (p *ReadPacket) GetHandle() string {
	return p.Handle
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ReadPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeRead, reqid)
	buf.AppendString(p.Handle)
	buf.AppendUint64(p.Offset)
	buf.AppendUint32(p.Length)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ReadPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = ReadPacket{
		Handle: buf.ConsumeString(),
		Offset: buf.ConsumeUint64(),
		Length: buf.ConsumeUint32(),
	}

	return buf.Err
}

// WritePacket defines the SSH_FXP_WRITE packet.
type WritePacket struct {
	Handle string
	Offset uint64
	Data   []byte
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *WritePacket) Type() PacketType {
	return PacketTypeWrite
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *WritePacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(handle) + uint64(offset) + bytes(data)
	return 1 + 4 + 4 + len(p.Handle) + 8 + 4 + len(p.Data)
}

// GetHandle returns the handle field of the packet.
func (p *WritePacket) GetHandle() string {
	return p.Handle
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *WritePacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		// exclude the data length, that will be carried in the separate payload.
		buf = NewMarshalBuffer(p.MarshalSize() - len(p.Data))
	}

	buf.StartPacket(PacketTypeWrite, reqid)
	buf.AppendString(p.Handle)
	buf.AppendUint64(p.Offset)
	buf.AppendUint32(uint32(len(p.Data)))

	return buf.Packet(p.Data)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
//
// If p.Data is already populated, and of sufficient length to hold the data,
// then this will copy the data into that byte slice.
//
// If p.Data has a length insufficient to hold the data,
// then this will make a new slice of sufficient length, and copy the data into that.
//
// This means this _does not_ alias any of the data buffer that is passed in.
func (p *WritePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	hint := p.Data

	*p = WritePacket{
		Handle: buf.ConsumeString(),
		Offset: buf.ConsumeUint64(),
		Data:   buf.ConsumeBytesCopy(hint),
	}

	return buf.Err
}

// FStatPacket defines the SSH_FXP_FSTAT packet.
type FStatPacket struct {
	Handle string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *FStatPacket) Type() PacketType {
	return PacketTypeFStat
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *FStatPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(handle)
	return 1 + 4 + 4 + len(p.Handle)
}

// GetHandle returns the handle field of the packet.
func (p *FStatPacket) GetHandle() string {
	return p.Handle
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *FStatPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeFStat, reqid)
	buf.AppendString(p.Handle)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *FStatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = FStatPacket{
		Handle: buf.ConsumeString(),
	}

	return buf.Err
}

// FSetStatPacket defines the SSH_FXP_FSETSTAT packet.
type FSetStatPacket struct {
	Handle string
	Attrs  Attributes
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *FSetStatPacket) Type() PacketType {
	return PacketTypeFSetStat
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *FSetStatPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(handle) ATTRS(attrs)
	return 1 + 4 + 4 + len(p.Handle) + p.Attrs.MarshalSize()
}

// GetHandle returns the handle field of the packet.
func (p *FSetStatPacket) GetHandle() string {
	return p.Handle
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *FSetStatPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeFSetStat, reqid)
	buf.AppendString(p.Handle)

	p.Attrs.MarshalInto(buf)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *FSetStatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = FSetStatPacket{
		Handle: buf.ConsumeString(),
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// ReadDirPacket defines the SSH_FXP_READDIR packet.
type ReadDirPacket struct {
	Handle string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *ReadDirPacket) Type() PacketType {
	return PacketTypeReadDir
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *ReadDirPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(handle)
	return 1 + 4 + 4 + len(p.Handle)
}

// GetHandle returns the handle field of the packet.
func (p *ReadDirPacket) GetHandle() string {
	return p.Handle
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ReadDirPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeReadDir, reqid)
	buf.AppendString(p.Handle)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ReadDirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = ReadDirPacket{
		Handle: buf.ConsumeString(),
	}

	return buf.Err
}
