package sftp

type SSHFxpRealpathPacket struct {
	ID   uint32
	Path string
}

func (p SSHFxpRealpathPacket) Id() uint32 { return p.ID }

func (p SSHFxpRealpathPacket) GetPath() string { return p.Path }

func (p SSHFxpRealpathPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_REALPATH, p.ID, p.Path)
}

func (p *SSHFxpRealpathPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}
