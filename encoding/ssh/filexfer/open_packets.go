package filexfer

// SSH_FXF_* flags.
const (
	FlagRead      = 1 << iota // SSH_FXF_READ
	FlagWrite                 // SSH_FXF_WRITE
	FlagAppend                // SSH_FXF_APPEND
	FlagCreate                // SSH_FXF_CREAT
	FlagTruncate              // SSH_FXF_TRUNC
	FlagExclusive             // SSH_FXF_EXCL
)

// OpenPacket defines the SSH_FXP_OPEN packet.
type OpenPacket struct {
	Filename string
	PFlags   uint32
	Attrs    Attributes
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *OpenPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	// string(filename) + uint32(pflags) + ATTRS(attrs)
	size := 4 + len(p.Filename) + 4 + p.Attrs.Len()

	b := NewMarshalBuffer(PacketTypeOpen, reqid, size)

	b.AppendString(p.Filename)
	b.AppendUint32(p.PFlags)

	p.Attrs.MarshalInto(b)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *OpenPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Filename, err = buf.ConsumeString(); err != nil {
		return err
	}

	if p.PFlags, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.Attrs.UnmarshalFrom(buf)
}

// OpenDirPacket defines the SSH_FXP_OPENDIR packet.
type OpenDirPacket struct {
	Path string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *OpenDirPacket) MarshalPacket(reqid uint32) (header, payload []byte, err error) {
	size := 4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(PacketTypeOpenDir, reqid, size)

	b.AppendString(p.Path)

	return b.Packet(payload)
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *OpenDirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}
