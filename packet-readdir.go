package sftp

type SSHFxpReaddirPacket struct {
	ID     uint32
	Handle string
}

func (p SSHFxpReaddirPacket) Id() uint32 { return p.ID }

func (p SSHFxpReaddirPacket) GetHandle() string { return p.Handle }

func (p SSHFxpReaddirPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_READDIR, p.ID, p.Handle)
}

func (p *SSHFxpReaddirPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Handle)
}

func (p *SSHFxpReaddirPacket) Accept(v RequestPacketVisitor) error {
	return v.VisitReaddirPacket(p)
}
