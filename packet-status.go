package sftp

type SSHFxpStatusPacket struct {
	ID uint32
	StatusError
}

func (p SSHFxpStatusPacket) Id() uint32 { return p.ID }

func (p SSHFxpStatusPacket) MarshalBinary() ([]byte, error) {
	b := []byte{ssh_FXP_STATUS}
	b = marshalUint32(b, p.ID)
	b = marshalStatus(b, p.StatusError)
	return b, nil
}
