package sftp

type SSHFxpRmdirPacket struct {
	ID   uint32
	Path string
}

func (p SSHFxpRmdirPacket) Id() uint32 { return p.ID }

func (p SSHFxpRmdirPacket) GetPath() string { return p.Path }

func (p SSHFxpRmdirPacket) NotReadOnly() {}

func (p SSHFxpRmdirPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_RMDIR, p.ID, p.Path)
}

func (p *SSHFxpRmdirPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

func (p *SSHFxpRmdirPacket) Accept(v RequestPacketVisitor) error {
	return v.VisitRmdirPacket(p)
}
