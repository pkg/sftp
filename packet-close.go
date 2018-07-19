package sftp

type SSHFxpClosePacket struct {
	ID     uint32
	Handle string
}

func (p SSHFxpClosePacket) Id() uint32 { return p.ID }

func (p SSHFxpClosePacket) GetHandle() string { return p.Handle }

func (p SSHFxpClosePacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_CLOSE, p.ID, p.Handle)
}

func (p *SSHFxpClosePacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Handle)
}
