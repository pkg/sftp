package sftp

type SSHFxpPosixRenamePacket struct {
	ID      uint32
	Oldpath string
	Newpath string
}

func (p SSHFxpPosixRenamePacket) Id() uint32 { return p.ID }

func (p SSHFxpPosixRenamePacket) GetPath() string { return p.Oldpath }

func (p SSHFxpPosixRenamePacket) Readonly() bool { return false }

func (p SSHFxpPosixRenamePacket) NotReadOnly() {}

func (p SSHFxpPosixRenamePacket) MarshalBinary() ([]byte, error) {
	const ext = "posix-rename@openssh.com"
	l := 1 + 4 + // type(byte) + uint32
		4 + len(ext) +
		4 + len(p.Oldpath) +
		4 + len(p.Newpath)

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_EXTENDED)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, ext)
	b = marshalString(b, p.Oldpath)
	b = marshalString(b, p.Newpath)
	return b, nil
}
