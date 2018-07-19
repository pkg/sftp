package sftp

type SSHFxpOpenPacket struct {
	ID     uint32
	Path   string
	Pflags uint32
	Flags  uint32 // ignored
}

func (p SSHFxpOpenPacket) Id() uint32 { return p.ID }

func (p SSHFxpOpenPacket) GetPath() string { return p.Path }

func (p SSHFxpOpenPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 +
		4 + len(p.Path) +
		4 + 4

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_OPEN)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Path)
	b = marshalUint32(b, p.Pflags)
	b = marshalUint32(b, p.Flags)
	return b, nil
}

func (p *SSHFxpOpenPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Path, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Pflags, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Flags, _, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	return nil
}

func (p SSHFxpOpenPacket) Readonly() bool {
	return !p.HasPflags(ssh_FXF_WRITE)
}

func (p SSHFxpOpenPacket) HasPflags(flags ...uint32) bool {
	for _, f := range flags {
		if p.Pflags&f == 0 {
			return false
		}
	}
	return true
}

func (p *SSHFxpOpenPacket) Accept(v RequestPacketVisitor) error {
	return v.VisitOpenPacket(p)
}
