package sftp

type SSHFxpExtendedPacketPosixRename struct {
	ID              uint32
	ExtendedRequest string
	Oldpath         string
	Newpath         string
}

func (p SSHFxpExtendedPacketPosixRename) Id() uint32 { return p.ID }

func (p SSHFxpExtendedPacketPosixRename) GetPath() string { return p.Oldpath }

func (p SSHFxpExtendedPacketPosixRename) Readonly() bool { return false }

func (p SSHFxpExtendedPacketPosixRename) NotReadOnly() {}

func (p *SSHFxpExtendedPacketPosixRename) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.ExtendedRequest, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Oldpath, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Newpath, _, err = unmarshalStringSafe(b); err != nil {
		return err
	}
	return nil
}
