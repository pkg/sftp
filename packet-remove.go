package sftp

type SSHFxpRemovePacket struct {
	ID       uint32
	Filename string
}

func (p SSHFxpRemovePacket) Id() uint32 { return p.ID }

func (p SSHFxpRemovePacket) GetPath() string { return p.Filename }

func (p SSHFxpRemovePacket) NotReadOnly() {}

func (p SSHFxpRemovePacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_REMOVE, p.ID, p.Filename)
}

func (p *SSHFxpRemovePacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Filename)
}

func (p *SSHFxpRemovePacket) Accept(v RequestPacketVisitor) error {
	return v.VisitRemovePacket(p)
}
