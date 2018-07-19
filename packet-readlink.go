package sftp

type SSHFxpReadlinkPacket struct {
	ID   uint32
	Path string
}

func (p SSHFxpReadlinkPacket) Id() uint32 { return p.ID }

func (p SSHFxpReadlinkPacket) GetPath() string { return p.Path }

func (p SSHFxpReadlinkPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_READLINK, p.ID, p.Path)
}

func (p *SSHFxpReadlinkPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

func (p *SSHFxpReadlinkPacket) Accept(v RequestPacketVisitor) error {
	return v.VisitReadlinkPacket(p)
}
