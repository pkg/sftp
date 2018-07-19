package sftp

type SSHFxpNameAttr struct {
	Name     string
	LongName string
	Attrs    []interface{}
}

func (p SSHFxpNameAttr) MarshalBinary() ([]byte, error) {
	b := []byte{}
	b = marshalString(b, p.Name)
	b = marshalString(b, p.LongName)
	for _, attr := range p.Attrs {
		b = marshal(b, attr)
	}
	return b, nil
}

type SSHFxpNamePacket struct {
	ID        uint32
	NameAttrs []SSHFxpNameAttr
}

func (p SSHFxpNamePacket) Id() uint32 { return p.ID }

func (p SSHFxpNamePacket) MarshalBinary() ([]byte, error) {
	b := []byte{}
	b = append(b, ssh_FXP_NAME)
	b = marshalUint32(b, p.ID)
	b = marshalUint32(b, uint32(len(p.NameAttrs)))
	for _, na := range p.NameAttrs {
		ab, err := na.MarshalBinary()
		if err != nil {
			return nil, err
		}

		b = append(b, ab...)
	}
	return b, nil
}
