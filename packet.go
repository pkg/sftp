package sftp

import (
	"encoding"
	"encoding/hex"
	"fmt"
	"io"
	"reflect"
)

var shortPacketError = fmt.Errorf("packet too short")

func marshalUint32(b []byte, v uint32) []byte {
	return append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func marshalUint64(b []byte, v uint64) []byte {
	return marshalUint32(marshalUint32(b, uint32(v>>32)), uint32(v))
}

func marshalString(b []byte, v string) []byte {
	return append(marshalUint32(b, uint32(len(v))), v...)
}

func marshal(b []byte, v interface{}) []byte {
	switch v := v.(type) {
	case uint8:
		return append(b, v)
	case uint32:
		return marshalUint32(b, v)
	case uint64:
		return marshalUint64(b, v)
	case string:
		return marshalString(b, v)
	default:
		switch d := reflect.ValueOf(v); d.Kind() {
		case reflect.Struct:
			for i, n := 0, d.NumField(); i < n; i++ {
				b = append(marshal(b, d.Field(i).Interface()))
			}
			return b
		case reflect.Slice:
			for i, n := 0, d.Len(); i < n; i++ {
				b = append(marshal(b, d.Index(i).Interface()))
			}
			return b
		default:
			panic(fmt.Sprintf("marshal(%#v): cannot handle type %T", v, v))
		}
	}
}

func unmarshalUint32(b []byte) (uint32, []byte) {
	v := uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
	return v, b[4:]
}

func unmarshalUint32Safe(b []byte) (uint32, []byte, error) {
	if len(b) < 4 {
		return 0, nil, shortPacketError
	}
	v := uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
	return v, b[4:], nil
}

func unmarshalUint64(b []byte) (uint64, []byte) {
	h, b := unmarshalUint32(b)
	l, b := unmarshalUint32(b)
	return uint64(h)<<32 | uint64(l), b
}

func unmarshalUint64Safe(b []byte) (uint64, []byte, error) {
	if len(b) < 8 {
		return 0, nil, shortPacketError
	}
	h, b := unmarshalUint32(b)
	l, b := unmarshalUint32(b)
	return uint64(h)<<32 | uint64(l), b, nil
}

func unmarshalString(b []byte) (string, []byte) {
	n, b := unmarshalUint32(b)
	return string(b[:n]), b[n:]
}

func unmarshalStringSafe(b []byte) (string, []byte, error) {
	n, b, err := unmarshalUint32Safe(b)
	if err != nil {
		return "", nil, err
	}
	if int64(n) > int64(len(b)) {
		return "", nil, shortPacketError
	}
	return string(b[:n]), b[n:], nil
}

// sendPacket marshals p according to RFC 4234.

func sendPacket(w io.Writer, m encoding.BinaryMarshaler) error {
	bb, err := m.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal2(%#v): binary marshaller failed", err)
	}
	l := uint32(len(bb))
	hdr := []byte{byte(l >> 24), byte(l >> 16), byte(l >> 8), byte(l)}
	debug("send packet %T, len: %v data: %v", m, l, hex.EncodeToString(bb))
	_, err = w.Write(hdr)
	if err != nil {
		return err
	}
	_, err = w.Write(bb)
	return err
}

func (svr *Server) sendPacket(m encoding.BinaryMarshaler) error {
	return sendPacket(svr.out, m)
}

func recvPacket(r io.Reader) (uint8, []byte, error) {
	var b = []byte{0, 0, 0, 0}
	if _, err := io.ReadFull(r, b); err != nil {
		return 0, nil, err
	}
	l, _ := unmarshalUint32(b)
	debug("recv packet %d bytes", l)
	b = make([]byte, l)
	if _, err := io.ReadFull(r, b); err != nil {
		debug("recv packet %d bytes: err %v", l, err)
		return 0, nil, err
	}
	debug("recv packet %d bytes: %v", l, hex.EncodeToString(b))
	return b[0], b[1:], nil
}

type ExtensionPair struct {
	Name string
	Data string
}

func unmarshalExtensionPair(b []byte) (ExtensionPair, []byte, error) {
	ep := ExtensionPair{}
	var err error = nil
	ep.Name, b, err = unmarshalStringSafe(b)
	if err != nil {
		return ep, b, err
	}
	ep.Data, b, err = unmarshalStringSafe(b)
	if err != nil {
		return ep, b, err
	}
	return ep, b, err
}

// Here starts the definition of packets along with their MarshalBinary
// implementations.
// Manually writing the marshalling logic wins us a lot of time and
// allocation.

type sshFxInitPacket struct {
	Version    uint32
	Extensions []ExtensionPair
}

func (p sshFxInitPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 // byte + uint32
	for _, e := range p.Extensions {
		l += 4 + len(e.Name) + 4 + len(e.Data)
	}

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_INIT)
	b = marshalUint32(b, p.Version)
	for _, e := range p.Extensions {
		b = marshalString(b, e.Name)
		b = marshalString(b, e.Data)
	}
	return b, nil
}

func (p *sshFxInitPacket) UnmarshalBinary(b []byte) (err error) {
	if p.Version, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	for len(b) > 0 {
		ep := ExtensionPair{}
		ep, b, err = unmarshalExtensionPair(b)
		if err != nil {
			return err
		}
		p.Extensions = append(p.Extensions, ep)
	}
	return nil
}

type sshFxVersionPacket struct {
	Version    uint32
	Extensions []struct {
		Name, Data string
	}
}

func (p sshFxVersionPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 // byte + uint32
	for _, e := range p.Extensions {
		l += 4 + len(e.Name) + 4 + len(e.Data)
	}

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_VERSION)
	b = marshalUint32(b, p.Version)
	for _, e := range p.Extensions {
		b = marshalString(b, e.Name)
		b = marshalString(b, e.Data)
	}
	return b, nil
}

func marshalIdString(packetType byte, id uint32, str string) ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(str)

	b := make([]byte, 0, l)
	b = append(b, packetType)
	b = marshalUint32(b, id)
	b = marshalString(b, str)
	return b, nil
}

func unmarshalIdString(b []byte, id *uint32, str *string) (err error) {
	*id, b, err = unmarshalUint32Safe(b)
	if err != nil {
		return
	}
	*str, b, err = unmarshalStringSafe(b)
	if err != nil {
		return
	}
	return
}

type sshFxpReaddirPacket struct {
	Id     uint32
	Handle string
}

func (p sshFxpReaddirPacket) MarshalBinary() ([]byte, error) {
	return marshalIdString(ssh_FXP_READDIR, p.Id, p.Handle)
}

type sshFxpOpendirPacket struct {
	Id   uint32
	Path string
}

func (p sshFxpOpendirPacket) MarshalBinary() ([]byte, error) {
	return marshalIdString(ssh_FXP_OPENDIR, p.Id, p.Path)
}

type sshFxpLstatPacket struct {
	Id   uint32
	Path string
}

func (p sshFxpLstatPacket) MarshalBinary() ([]byte, error) {
	return marshalIdString(ssh_FXP_LSTAT, p.Id, p.Path)
}

func (p *sshFxpLstatPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIdString(b, &p.Id, &p.Path)
}

type sshFxpFstatPacket struct {
	Id     uint32
	Handle string
}

func (p sshFxpFstatPacket) MarshalBinary() ([]byte, error) {
	return marshalIdString(ssh_FXP_FSTAT, p.Id, p.Handle)
}

type sshFxpClosePacket struct {
	Id     uint32
	Handle string
}

func (p sshFxpClosePacket) MarshalBinary() ([]byte, error) {
	return marshalIdString(ssh_FXP_CLOSE, p.Id, p.Handle)
}

func (p *sshFxpClosePacket) UnmarshalBinary(b []byte) error {
	return unmarshalIdString(b, &p.Id, &p.Handle)
}

type sshFxpRemovePacket struct {
	Id       uint32
	Filename string
}

func (p sshFxpRemovePacket) MarshalBinary() ([]byte, error) {
	return marshalIdString(ssh_FXP_REMOVE, p.Id, p.Filename)
}

type sshFxpRmdirPacket struct {
	Id   uint32
	Path string
}

func (p sshFxpRmdirPacket) MarshalBinary() ([]byte, error) {
	return marshalIdString(ssh_FXP_RMDIR, p.Id, p.Path)
}

type sshFxpReadlinkPacket struct {
	Id   uint32
	Path string
}

func (p sshFxpReadlinkPacket) MarshalBinary() ([]byte, error) {
	return marshalIdString(ssh_FXP_READLINK, p.Id, p.Path)
}

type sshFxpOpenPacket struct {
	Id     uint32
	Path   string
	Pflags uint32
	Flags  uint32 // ignored
}

func (p sshFxpOpenPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 +
		4 + len(p.Path) +
		4 + 4

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_OPEN)
	b = marshalUint32(b, p.Id)
	b = marshalString(b, p.Path)
	b = marshalUint32(b, p.Pflags)
	b = marshalUint32(b, p.Flags)
	return b, nil
}

func (p *sshFxpOpenPacket) UnmarshalBinary(b []byte) (err error) {
	if p.Id, b, err = unmarshalUint32Safe(b); err != nil {
		return
	} else if p.Path, b, err = unmarshalStringSafe(b); err != nil {
		return
	} else if p.Pflags, b, err = unmarshalUint32Safe(b); err != nil {
		return
	} else if p.Flags, b, err = unmarshalUint32Safe(b); err != nil {
		return
	}
	return
}

type sshFxpReadPacket struct {
	Id     uint32
	Handle string
	Offset uint64
	Len    uint32
}

func (p sshFxpReadPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Handle) +
		8 + 4 // uint64 + uint32

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_READ)
	b = marshalUint32(b, p.Id)
	b = marshalString(b, p.Handle)
	b = marshalUint64(b, p.Offset)
	b = marshalUint32(b, p.Len)
	return b, nil
}

type sshFxpRenamePacket struct {
	Id      uint32
	Oldpath string
	Newpath string
}

func (p sshFxpRenamePacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Oldpath) +
		4 + len(p.Newpath)

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_RENAME)
	b = marshalUint32(b, p.Id)
	b = marshalString(b, p.Oldpath)
	b = marshalString(b, p.Newpath)
	return b, nil
}

type sshFxpWritePacket struct {
	Id     uint32
	Handle string
	Offset uint64
	Length uint32
	Data   []byte
}

func (s sshFxpWritePacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(s.Handle) +
		8 + 4 + // uint64 + uint32
		len(s.Data)

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_WRITE)
	b = marshalUint32(b, s.Id)
	b = marshalString(b, s.Handle)
	b = marshalUint64(b, s.Offset)
	b = marshalUint32(b, s.Length)
	b = append(b, s.Data...)
	return b, nil
}

type sshFxpMkdirPacket struct {
	Id    uint32
	Path  string
	Flags uint32 // ignored
}

func (p sshFxpMkdirPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Path) +
		4 // uint32

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_MKDIR)
	b = marshalUint32(b, p.Id)
	b = marshalString(b, p.Path)
	b = marshalUint32(b, p.Flags)
	return b, nil
}

func (p *sshFxpMkdirPacket) UnmarshalBinary(b []byte) (err error) {
	if p.Id, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Path, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Flags, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	return nil
}

type sshFxpSetstatPacket struct {
	Id    uint32
	Path  string
	Flags uint32
	Attrs interface{}
}

func (p sshFxpSetstatPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Path) +
		4 // uint32 + uint64

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_SETSTAT)
	b = marshalUint32(b, p.Id)
	b = marshalString(b, p.Path)
	b = marshalUint32(b, p.Flags)
	b = marshal(b, p.Attrs)
	return b, nil
}

type sshFxpHandlePacket struct {
	Id     uint32
	Handle string
}

func (p sshFxpHandlePacket) MarshalBinary() ([]byte, error) {
	b := []byte{ssh_FXP_HANDLE}
	b = marshalUint32(b, p.Id)
	b = marshalString(b, p.Handle)
	return b, nil
}

type sshFxpStatusPacket struct {
	Id uint32
	StatusError
}

func (p sshFxpStatusPacket) MarshalBinary() ([]byte, error) {
	b := []byte{ssh_FXP_STATUS}
	b = marshalUint32(b, p.Id)
	b = marshalStatus(b, p.StatusError)
	return b, nil
}
