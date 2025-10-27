package sshfx

// LStatPacket defines the SSH_FXP_LSTAT packet.
type LStatPacket struct {
	Path string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *LStatPacket) Type() PacketType {
	return PacketTypeLStat
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *LStatPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(path)
	return 1 + 4 + 4 + len(p.Path)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *LStatPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeLStat, reqid)
	buf.AppendString(p.Path)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *LStatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = LStatPacket{
		Path: buf.ConsumeString(),
	}

	return buf.Err
}

// SetStatPacket defines the SSH_FXP_SETSTAT packet.
type SetStatPacket struct {
	Path  string
	Attrs Attributes
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *SetStatPacket) Type() PacketType {
	return PacketTypeSetStat
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *SetStatPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(path) + ATTRS(attrs)
	return 1 + 4 + 4 + len(p.Path) + p.Attrs.MarshalSize()
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *SetStatPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeSetStat, reqid)
	buf.AppendString(p.Path)

	p.Attrs.MarshalInto(buf)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *SetStatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = SetStatPacket{
		Path: buf.ConsumeString(),
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// RemovePacket defines the SSH_FXP_REMOVE packet.
type RemovePacket struct {
	Path string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *RemovePacket) Type() PacketType {
	return PacketTypeRemove
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *RemovePacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(path)
	return 1 + 4 + 4 + len(p.Path)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RemovePacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeRemove, reqid)
	buf.AppendString(p.Path)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RemovePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = RemovePacket{
		Path: buf.ConsumeString(),
	}

	return buf.Err
}

// MkdirPacket defines the SSH_FXP_MKDIR packet.
type MkdirPacket struct {
	Path  string
	Attrs Attributes
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *MkdirPacket) Type() PacketType {
	return PacketTypeMkdir
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *MkdirPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(path) + ATTRS(attrs)
	return 1 + 4 + 4 + len(p.Path) + p.Attrs.MarshalSize()
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *MkdirPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeMkdir, reqid)
	buf.AppendString(p.Path)

	p.Attrs.MarshalInto(buf)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *MkdirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = MkdirPacket{
		Path: buf.ConsumeString(),
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// RmdirPacket defines the SSH_FXP_RMDIR packet.
type RmdirPacket struct {
	Path string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *RmdirPacket) Type() PacketType {
	return PacketTypeRmdir
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *RmdirPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(path)
	return 1 + 4 + 4 + len(p.Path)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RmdirPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeRmdir, reqid)
	buf.AppendString(p.Path)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RmdirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = RmdirPacket{
		Path: buf.ConsumeString(),
	}

	return buf.Err
}

// RealPathPacket defines the SSH_FXP_REALPATH packet.
type RealPathPacket struct {
	Path string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *RealPathPacket) Type() PacketType {
	return PacketTypeRealPath
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *RealPathPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(path)
	return 1 + 4 + 4 + len(p.Path)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RealPathPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeRealPath, reqid)
	buf.AppendString(p.Path)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RealPathPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = RealPathPacket{
		Path: buf.ConsumeString(),
	}

	return buf.Err
}

// StatPacket defines the SSH_FXP_STAT packet.
type StatPacket struct {
	Path string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *StatPacket) Type() PacketType {
	return PacketTypeStat
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *StatPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(path)
	return 1 + 4 + 4 + len(p.Path)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *StatPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeStat, reqid)
	buf.AppendString(p.Path)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *StatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = StatPacket{
		Path: buf.ConsumeString(),
	}

	return buf.Err
}

// RenamePacket defines the SSH_FXP_RENAME packet.
type RenamePacket struct {
	OldPath string
	NewPath string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *RenamePacket) Type() PacketType {
	return PacketTypeRename
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *RenamePacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(oldpath) + string(newpath)
	return 1 + 4 + 4 + len(p.OldPath) + 4 + len(p.NewPath)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RenamePacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeRename, reqid)
	buf.AppendString(p.OldPath)
	buf.AppendString(p.NewPath)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RenamePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = RenamePacket{
		OldPath: buf.ConsumeString(),
		NewPath: buf.ConsumeString(),
	}

	return buf.Err
}

// ReadLinkPacket defines the SSH_FXP_READLINK packet.
type ReadLinkPacket struct {
	Path string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *ReadLinkPacket) Type() PacketType {
	return PacketTypeReadLink
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *ReadLinkPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(path)
	return 1 + 4 + 4 + len(p.Path)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ReadLinkPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeReadLink, reqid)
	buf.AppendString(p.Path)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ReadLinkPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = ReadLinkPacket{
		Path: buf.ConsumeString(),
	}

	return buf.Err
}

// SymlinkPacket defines the SSH_FXP_SYMLINK packet.
//
// The order of the arguments to the SSH_FXP_SYMLINK method was inadvertently reversed.
// Unfortunately, the reversal was not noticed until the server was widely deployed.
// Covered in Section 4.1 of https://github.com/openssh/openssh-portable/blob/master/PROTOCOL
type SymlinkPacket struct {
	LinkPath   string
	TargetPath string
}

// Type returns the SSH_FXP_xy value associated with this packet type.
func (p *SymlinkPacket) Type() PacketType {
	return PacketTypeSymlink
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (p *SymlinkPacket) MarshalSize() int {
	// uint8(type) + uint32(request-id) + string(linkpath) + string(targetpath)
	return 1 + 4 + 4 + len(p.LinkPath) + 4 + len(p.TargetPath)
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *SymlinkPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	buf := NewBuffer(b)
	if buf.Cap() < 9 {
		buf = NewMarshalBuffer(p.MarshalSize())
	}

	buf.StartPacket(PacketTypeSymlink, reqid)

	// Arguments were inadvertently reversed.
	buf.AppendString(p.TargetPath)
	buf.AppendString(p.LinkPath)

	return buf.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *SymlinkPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	*p = SymlinkPacket{
		// Arguments were inadvertently reversed.
		TargetPath: buf.ConsumeString(),
		LinkPath:   buf.ConsumeString(),
	}

	return buf.Err
}
