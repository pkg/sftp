package sftp

type SSHFxpMkdirPacket struct {
	ID    uint32
	Path  string
	Flags uint32 // ignored
}

func (p SSHFxpMkdirPacket) Id() uint32 { return p.ID }

func (p SSHFxpMkdirPacket) GetPath() string { return p.Path }

func (p SSHFxpMkdirPacket) NotReadOnly() {}

func (p SSHFxpMkdirPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Path) +
		4 // uint32

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_MKDIR)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Path)
	b = marshalUint32(b, p.Flags)
	return b, nil
}

func (p *SSHFxpMkdirPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Path, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Flags, _, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	return nil
}

func (p *SSHFxpMkdirPacket) Accept(v RequestPacketVisitor) error {
	return v.VisitMkdirPacket(p)
}
