package filexfer

// LstatPacket defines the SSH_FXP_LSTAT packet.
type LstatPacket struct {
	Path string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *LstatPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(PacketTypeLstat, reqid, size)

	b.AppendString(p.Path)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *LstatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// SetstatPacket defines the SSH_FXP_SETSTAT packet.
type SetstatPacket struct {
	Path  string
	Attrs Attributes
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *SetstatPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Path) + p.Attrs.Len() // string(path) + ATTRS(attrs)

	b := NewMarshalBuffer(PacketTypeSetstat, reqid, size)

	b.AppendString(p.Path)

	p.Attrs.MarshalInto(b)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *SetstatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// RemovePacket defines the SSH_FXP_REMOVE packet.
type RemovePacket struct {
	Path string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RemovePacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(PacketTypeRemove, reqid, size)

	b.AppendString(p.Path)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RemovePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// MkdirPacket defines the SSH_FXP_MKDIR packet.
type MkdirPacket struct {
	Path  string
	Attrs Attributes
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *MkdirPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Path) + p.Attrs.Len() // string(path) + ATTRS(attrs)

	b := NewMarshalBuffer(PacketTypeMkdir, reqid, size)

	b.AppendString(p.Path)

	p.Attrs.MarshalInto(b)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *MkdirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// RmdirPacket defines the SSH_FXP_RMDIR packet.
type RmdirPacket struct {
	Path string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RmdirPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(PacketTypeRmdir, reqid, size)

	b.AppendString(p.Path)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RmdirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// RealpathPacket defines the SSH_FXP_REALPATH packet.
type RealpathPacket struct {
	Path string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RealpathPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(PacketTypeRealpath, reqid, size)

	b.AppendString(p.Path)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RealpathPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// StatPacket defines the SSH_FXP_STAT packet.
type StatPacket struct {
	Path string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *StatPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(PacketTypeStat, reqid, size)

	b.AppendString(p.Path)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *StatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// RenamePacket defines the SSH_FXP_RENAME packet.
type RenamePacket struct {
	OldPath string
	NewPath string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RenamePacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	// string(oldpath) + string(newpath)
	size := 4 + len(p.OldPath) + 4 + len(p.NewPath)

	b := NewMarshalBuffer(PacketTypeRename, reqid, size)

	b.AppendString(p.OldPath)
	b.AppendString(p.NewPath)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RenamePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.OldPath, err = buf.ConsumeString(); err != nil {
		return err
	}

	if p.NewPath, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// ReadlinkPacket defines the SSH_FXP_READLINK packet.
type ReadlinkPacket struct {
	Path string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ReadlinkPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(PacketTypeReadlink, reqid, size)

	b.AppendString(p.Path)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ReadlinkPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// SymlinkPacket defines the SSH_FXP_SYMLINK packet.
//
// The order of the arguments to the SSH_FXP_SYMLINK method was inadvertently reversed.
// Unfortunately, the reversal was not noticed until the server was widely deployed.
// Covered in Section 3.1 of https://github.com/openssh/openssh-portable/blob/master/PROTOCOL
type SymlinkPacket struct {
	LinkPath   string
	TargetPath string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *SymlinkPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	// string(targetpath) + string(linkpath)
	size := 4 + len(p.TargetPath) + 4 + len(p.LinkPath)

	b := NewMarshalBuffer(PacketTypeSymlink, reqid, size)

	// Arguments were inadvertently reversed.
	b.AppendString(p.TargetPath)
	b.AppendString(p.LinkPath)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *SymlinkPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	// Arguments were inadvertently reversed.
	if p.TargetPath, err = buf.ConsumeString(); err != nil {
		return err
	}

	if p.LinkPath, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}
