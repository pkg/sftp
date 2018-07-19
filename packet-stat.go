package sftp

import (
	"os"
)

type SSHFxpStatPacket struct {
	ID   uint32
	Path string
}

func (p SSHFxpStatPacket) Id() uint32 { return p.ID }

func (p SSHFxpStatPacket) GetPath() string { return p.Path }

func (p SSHFxpStatPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_STAT, p.ID, p.Path)
}

func (p *SSHFxpStatPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

func (p *SSHFxpStatPacket) Accept(v RequestPacketVisitor) error {
	return v.VisitStatPacket(p)
}

type SSHFxpFstatPacket struct {
	ID     uint32
	Handle string
}

func (p SSHFxpFstatPacket) Id() uint32 { return p.ID }

func (p SSHFxpFstatPacket) GetHandle() string { return p.Handle }

func (p SSHFxpFstatPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_FSTAT, p.ID, p.Handle)
}

func (p *SSHFxpFstatPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Handle)
}

func (p *SSHFxpFstatPacket) Accept(v RequestPacketVisitor) error {
	return v.VisitFstatPacket(p)
}

type SSHFxpLstatPacket struct {
	ID   uint32
	Path string
}

func (p SSHFxpLstatPacket) Id() uint32 { return p.ID }

func (p SSHFxpLstatPacket) GetPath() string { return p.Path }

func (p SSHFxpLstatPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(ssh_FXP_LSTAT, p.ID, p.Path)
}

func (p *SSHFxpLstatPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

func (p *SSHFxpLstatPacket) Accept(v RequestPacketVisitor) error {
	return v.VisitLstatPacket(p)
}

type SSHFxpStatResponse struct {
	ID   uint32
	Info os.FileInfo
}

func (p SSHFxpStatResponse) Id() uint32 { return p.ID }

func (p SSHFxpStatResponse) MarshalBinary() ([]byte, error) {
	b := []byte{ssh_FXP_ATTRS}
	b = marshalUint32(b, p.ID)
	b = marshalFileInfo(b, p.Info)
	return b, nil
}
