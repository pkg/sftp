package sftp

type SSHFxpOpendirPacket struct {
	ID   uint32
	Path string
}

func (p SSHFxpOpendirPacket) Id() uint32 { return p.ID }

func (p SSHFxpOpendirPacket) GetPath() string { return p.Path }

func (p SSHFxpOpendirPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_OPENDIR, p.ID, p.Path)
}

func (p *SSHFxpOpendirPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}
