package sftp

type SSHFxpHandlePacket struct {
	ID     uint32
	Handle string
}

func (p SSHFxpHandlePacket) Id() uint32 { return p.ID }

func (p SSHFxpHandlePacket) MarshalBinary() ([]byte, error) {
	b := []byte{ssh_FXP_HANDLE}
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Handle)
	return b, nil
}
