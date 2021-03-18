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
		4 + len(p.Filename) + // string
		4 // uint32(pflags)

	b := NewMarshalBuffer(size)
	b.AppendUint8(uint8(PacketTypeOpen))
	b.AppendUint32(p.RequestID)
	b.AppendString(p.Filename)
	b.AppendUint32(p.PFlags)

	size += p.Attrs.MarshalInto(b)

	b.PutLength(size)

	return b.Bytes(), nil, nil
}

// MarshalBinary returns p as the binary encoding of p.
func (p *OpenPacket) MarshalBinary() ([]byte, error) {
	return ComposePacket(p.MarshalPacket())
}

// UnmarshalPacketBody unmarshals the packet body from the given Buffer.
func (p *OpenPacket) UnmarshalPacketBody(buf *Buffer) (err error) {
	if p.RequestID, err = buf.ConsumeUint32(); err != nil {
		return err
	}

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
func (p *OpenPacket) UnmarshalBinary(data []byte) error {
	return p.UnmarshalPacketBody(NewBuffer(data))
}
