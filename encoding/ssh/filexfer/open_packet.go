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
	RequestID uint32
	Filename  string
	PFlags    uint32
	Attrs     Attributes
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *OpenPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Filename) + 4 + // string(filename) + uint32(pflags)
		4 // minimum marshal size of Attributes

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeOpen))
	b.AppendUint32(p.RequestID)
	b.AppendString(p.Filename)
	b.AppendUint32(p.PFlags)

	p.Attrs.MarshalInto(b)

	b.PutLength(b.Len() - 4)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *OpenPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
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

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *OpenPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}

// OpendirPacket defines the SSH_FXP_OPEN packet.
type OpendirPacket struct {
	RequestID uint32
	Path      string
}

// MarshalPacket returns p as a two-part binary encoding of p.
func (p *OpendirPacket) MarshalPacket() (header, payload []byte, err error) {
	size := 1 + 4 + // byte(type) + uint32(request-id)
		4 + len(p.Path) // string(path)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeOpendir))
	b.AppendUint32(p.RequestID)
	b.AppendString(p.Path)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *OpendirPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
// It is assumed that the uint32(request-id) has already been consumed.
func (p *OpendirPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.Path, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary unmarshals a full raw packet out of the given data.
// It is assumed that the uint32(length) has already been consumed to receive the data.
// It is also assumed that the uint8(type) has already been consumed to which packet to unmarshal into.
func (p *OpendirPacket) UnmarshalBinary(data []byte) (err error) {
	buf := NewBuffer(data)

	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

	return p.UnmarshalPacketBody(buf)
}
