package filexfer

// LstatPacket defines the SSH_FXP_LSTAT packet.
type LstatPacket struct {
	RequestID uint32
	Path      string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *LstatPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeLstat))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.Path)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *LstatPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *LstatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *LstatPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// SetstatPacket defines the SSH_FXP_SETSTAT packet.
type SetstatPacket struct {
	RequestID uint32
	Path      string
	Attrs     Attributes
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *SetstatPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Path) + // string(filename) + uint32(pflags)
		4 // minimum marshal size of Attributes

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeSetstat))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.Path)

	p.Attrs.MarshalInto(b)

	b.PutLength(b.Len() - 4)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *SetstatPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *SetstatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *SetstatPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// RemovePacket defines the SSH_FXP_REMOVE packet.
type RemovePacket struct {
	RequestID uint32
	Path      string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RemovePacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeRemove))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.Path)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *RemovePacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RemovePacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *RemovePacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// MkdirPacket defines the SSH_FXP_MKDIR packet.
type MkdirPacket struct {
	RequestID uint32
	Path      string
	Attrs     Attributes
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *MkdirPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Path) + // string(filename) + uint32(pflags)
		4 // minimum marshal size of Attributes

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeMkdir))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.Path)

	p.Attrs.MarshalInto(b)

	b.PutLength(b.Len() - 4)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *MkdirPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *MkdirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *MkdirPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// RmdirPacket defines the SSH_FXP_RMDIR packet.
type RmdirPacket struct {
	RequestID uint32
	Path      string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RmdirPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeRmdir))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.Path)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *RmdirPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RmdirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *RmdirPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// RealpathPacket defines the SSH_FXP_REALPATH packet.
type RealpathPacket struct {
	RequestID uint32
	Path      string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RealpathPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeRealpath))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.Path)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *RealpathPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *RealpathPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *RealpathPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// StatPacket defines the SSH_FXP_STAT packet.
type StatPacket struct {
	RequestID uint32
	Path      string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *StatPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeStat))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.Path)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *StatPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *StatPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *StatPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// RenamePacket defines the SSH_FXP_RENAME packet.
type RenamePacket struct {
	RequestID uint32
	OldPath   string
	NewPath   string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *RenamePacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.OldPath) + // string(oldpath)
		4 + len(p.NewPath) // string(newpath)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeRename))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.OldPath)
	b.AppendString(p.NewPath)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *RenamePacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
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

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *RenamePacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// ReadlinkPacket defines the SSH_FXP_READLINK packet.
type ReadlinkPacket struct {
	RequestID uint32
	Path      string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *ReadlinkPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeReadlink))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.Path)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *ReadlinkPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *ReadlinkPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *ReadlinkPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// SymlinkPacket defines the SSH_FXP_SYMLINK packet.
//
// The order of the arguments to the SSH_FXP_SYMLINK method was inadvertently reversed.
// Unfortunately, the reversal was not noticed until the server was widely deployed.
// Covered in Section 3.1 of https://github.com/openssh/openssh-portable/blob/master/PROTOCOL
type SymlinkPacket struct {
	RequestID  uint32
	LinkPath   string
	TargetPath string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *SymlinkPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.TargetPath) + // string(targetpath)
		4 + len(p.LinkPath) // string(linkpath)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeSymlink))
	b.AppendUint32(p.RequestID)

	b.AppendString(p.TargetPath) // Arguments were inadvertently reversed.
	b.AppendString(p.LinkPath)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *SymlinkPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
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

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *SymlinkPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}
