package sftp

import (
	"encoding"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
)

var (
	errShortPacket = errors.New("packet too short")
)

const (
	debugDumpTxPacket      = false
	debugDumpRxPacket      = false
	debugDumpTxPacketBytes = false
	debugDumpRxPacketBytes = false
)

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
	if v == nil {
		return b
	}
	switch v := v.(type) {
	case uint8:
		return append(b, v)
	case uint32:
		return marshalUint32(b, v)
	case uint64:
		return marshalUint64(b, v)
	case string:
		return marshalString(b, v)
	case os.FileInfo:
		return marshalFileInfo(b, v)
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
	var v uint32
	if len(b) < 4 {
		return 0, nil, errShortPacket
	}
	v, b = unmarshalUint32(b)
	return v, b, nil
}

func unmarshalUint64(b []byte) (uint64, []byte) {
	h, b := unmarshalUint32(b)
	l, b := unmarshalUint32(b)
	return uint64(h)<<32 | uint64(l), b
}

func unmarshalUint64Safe(b []byte) (uint64, []byte, error) {
	var v uint64
	if len(b) < 8 {
		return 0, nil, errShortPacket
	}
	v, b = unmarshalUint64(b)
	return v, b, nil
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
		return "", nil, errShortPacket
	}
	return string(b[:n]), b[n:], nil
}

// sendPacket marshals p according to RFC 4234.
func sendPacket(w io.Writer, m encoding.BinaryMarshaler) error {
	bb, err := m.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal2(%#v): binary marshaller failed", err)
	}
	if debugDumpTxPacketBytes {
		debug("send packet: %s %d bytes %x", Fxpkt(bb[0]), len(bb), bb[1:])
	} else if debugDumpTxPacket {
		debug("send packet: %s %d bytes", Fxpkt(bb[0]), len(bb))
	}
	l := uint32(len(bb))
	hdr := []byte{byte(l >> 24), byte(l >> 16), byte(l >> 8), byte(l)}
	_, err = w.Write(hdr)
	if err != nil {
		return err
	}
	_, err = w.Write(bb)
	return err
}

func (svr *Server) SendPacket(m encoding.BinaryMarshaler) error {
	// any responder can call sendPacket(); actual socket access must be serialized
	svr.outMutex.Lock()
	defer svr.outMutex.Unlock()
	return sendPacket(svr.out, m)
}

func recvPacket(r io.Reader) (uint8, []byte, error) {
	var b = []byte{0, 0, 0, 0}
	if _, err := io.ReadFull(r, b); err != nil {
		return 0, nil, err
	}
	l, _ := unmarshalUint32(b)
	b = make([]byte, l)
	if _, err := io.ReadFull(r, b); err != nil {
		debug("recv packet %d bytes: err %v", l, err)
		return 0, nil, err
	}
	if debugDumpRxPacketBytes {
		debug("recv packet: %s %d bytes %x", Fxpkt(b[0]), l, b[1:])
	} else if debugDumpRxPacket {
		debug("recv packet: %s %d bytes", Fxpkt(b[0]), l)
	}
	return b[0], b[1:], nil
}

type extensionPair struct {
	Name string
	Data string
}

func unmarshalExtensionPair(b []byte) (extensionPair, []byte, error) {
	var ep extensionPair
	var err error
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

type SshFxInitPacket struct {
	Version    uint32
	Extensions []extensionPair
}

func (p SshFxInitPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 // byte + uint32
	for _, e := range p.Extensions {
		l += 4 + len(e.Name) + 4 + len(e.Data)
	}

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_INIT)
	b = marshalUint32(b, p.Version)
	for _, e := range p.Extensions {
		b = marshalString(b, e.Name)
		b = marshalString(b, e.Data)
	}
	return b, nil
}

func (p *SshFxInitPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.Version, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	for len(b) > 0 {
		var ep extensionPair
		ep, b, err = unmarshalExtensionPair(b)
		if err != nil {
			return err
		}
		p.Extensions = append(p.Extensions, ep)
	}
	return nil
}

type SshFxVersionPacket struct {
	Version    uint32
	Extensions []struct {
		Name, Data string
	}
}

func (p SshFxVersionPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 // byte + uint32
	for _, e := range p.Extensions {
		l += 4 + len(e.Name) + 4 + len(e.Data)
	}

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_VERSION)
	b = marshalUint32(b, p.Version)
	for _, e := range p.Extensions {
		b = marshalString(b, e.Name)
		b = marshalString(b, e.Data)
	}
	return b, nil
}

func marshalIDString(packetType byte, id uint32, str string) ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(str)

	b := make([]byte, 0, l)
	b = append(b, packetType)
	b = marshalUint32(b, id)
	b = marshalString(b, str)
	return b, nil
}

func unmarshalIDString(b []byte, id *uint32, str *string) error {
	var err error
	*id, b, err = unmarshalUint32Safe(b)
	if err != nil {
		return err
	}
	*str, b, err = unmarshalStringSafe(b)
	if err != nil {
		return err
	}
	return nil
}

type SshFxpReaddirPacket struct {
	ID     uint32
	Handle string
}

func (p SshFxpReaddirPacket) id() uint32 { return p.ID }

func (p SshFxpReaddirPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_READDIR, p.ID, p.Handle)
}

func (p *SshFxpReaddirPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Handle)
}

type SshFxpOpendirPacket struct {
	ID   uint32
	Path string
}

func (p SshFxpOpendirPacket) id() uint32 { return p.ID }

func (p SshFxpOpendirPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_OPENDIR, p.ID, p.Path)
}

func (p *SshFxpOpendirPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

type SshFxpLstatPacket struct {
	ID   uint32
	Path string
}

func (p SshFxpLstatPacket) id() uint32 { return p.ID }

func (p SshFxpLstatPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_LSTAT, p.ID, p.Path)
}

func (p *SshFxpLstatPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

type SshFxpStatPacket struct {
	ID   uint32
	Path string
}

func (p SshFxpStatPacket) id() uint32 { return p.ID }

func (p SshFxpStatPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_STAT, p.ID, p.Path)
}

func (p *SshFxpStatPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

type SshFxpFstatPacket struct {
	ID     uint32
	Handle string
}

func (p SshFxpFstatPacket) id() uint32 { return p.ID }

func (p SshFxpFstatPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_FSTAT, p.ID, p.Handle)
}

func (p *SshFxpFstatPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Handle)
}

type SshFxpClosePacket struct {
	ID     uint32
	Handle string
}

func (p SshFxpClosePacket) id() uint32 { return p.ID }

func (p SshFxpClosePacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_CLOSE, p.ID, p.Handle)
}

func (p *SshFxpClosePacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Handle)
}

type SshFxpRemovePacket struct {
	ID       uint32
	Filename string
}

func (p SshFxpRemovePacket) id() uint32 { return p.ID }

func (p SshFxpRemovePacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_REMOVE, p.ID, p.Filename)
}

func (p *SshFxpRemovePacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Filename)
}

type SshFxpRmdirPacket struct {
	ID   uint32
	Path string
}

func (p SshFxpRmdirPacket) id() uint32 { return p.ID }

func (p SshFxpRmdirPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_RMDIR, p.ID, p.Path)
}

func (p *SshFxpRmdirPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

type SshFxpSymlinkPacket struct {
	ID         uint32
	Targetpath string
	Linkpath   string
}

func (p SshFxpSymlinkPacket) id() uint32 { return p.ID }

func (p SshFxpSymlinkPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Targetpath) +
		4 + len(p.Linkpath)

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_SYMLINK)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Targetpath)
	b = marshalString(b, p.Linkpath)
	return b, nil
}

func (p *SshFxpSymlinkPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Targetpath, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Linkpath, b, err = unmarshalStringSafe(b); err != nil {
		return err
	}
	return nil
}

type SshFxpReadlinkPacket struct {
	ID   uint32
	Path string
}

func (p SshFxpReadlinkPacket) id() uint32 { return p.ID }

func (p SshFxpReadlinkPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_READLINK, p.ID, p.Path)
}

func (p *SshFxpReadlinkPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

type SshFxpRealpathPacket struct {
	ID   uint32
	Path string
}

func (p SshFxpRealpathPacket) id() uint32 { return p.ID }

func (p SshFxpRealpathPacket) MarshalBinary() ([]byte, error) {
	return marshalIDString(SSH_FXP_REALPATH, p.ID, p.Path)
}

func (p *SshFxpRealpathPacket) UnmarshalBinary(b []byte) error {
	return unmarshalIDString(b, &p.ID, &p.Path)
}

type SshFxpNameAttr struct {
	Name     string
	LongName string
	Attrs    []interface{}
}

func (p SshFxpNameAttr) MarshalBinary() ([]byte, error) {
	b := []byte{}
	b = marshalString(b, p.Name)
	b = marshalString(b, p.LongName)
	for _, attr := range p.Attrs {
		b = marshal(b, attr)
	}
	return b, nil
}

type SshFxpNamePacket struct {
	ID        uint32
	NameAttrs []SshFxpNameAttr
}

func (p SshFxpNamePacket) MarshalBinary() ([]byte, error) {
	b := []byte{}
	b = append(b, SSH_FXP_NAME)
	b = marshalUint32(b, p.ID)
	b = marshalUint32(b, uint32(len(p.NameAttrs)))
	for _, na := range p.NameAttrs {
		ab, err := na.MarshalBinary()
		if err != nil {
			return nil, err
		}

		b = append(b, ab...)
	}
	return b, nil
}

type SshFxpOpenPacket struct {
	ID     uint32
	Path   string
	Pflags uint32
	Flags  uint32 // ignored
}

func (p SshFxpOpenPacket) id() uint32 { return p.ID }

func (p SshFxpOpenPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 +
		4 + len(p.Path) +
		4 + 4

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_OPEN)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Path)
	b = marshalUint32(b, p.Pflags)
	b = marshalUint32(b, p.Flags)
	return b, nil
}

func (p *SshFxpOpenPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Path, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Pflags, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Flags, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	return nil
}

type SshFxpReadPacket struct {
	ID     uint32
	Handle string
	Offset uint64
	Len    uint32
}

func (p SshFxpReadPacket) id() uint32 { return p.ID }

func (p SshFxpReadPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Handle) +
		8 + 4 // uint64 + uint32

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_READ)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Handle)
	b = marshalUint64(b, p.Offset)
	b = marshalUint32(b, p.Len)
	return b, nil
}

func (p *SshFxpReadPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Handle, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Offset, b, err = unmarshalUint64Safe(b); err != nil {
		return err
	} else if p.Len, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	return nil
}

type SshFxpRenamePacket struct {
	ID      uint32
	Oldpath string
	Newpath string
}

func (p SshFxpRenamePacket) id() uint32 { return p.ID }

func (p SshFxpRenamePacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Oldpath) +
		4 + len(p.Newpath)

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_RENAME)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Oldpath)
	b = marshalString(b, p.Newpath)
	return b, nil
}

func (p *SshFxpRenamePacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Oldpath, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Newpath, b, err = unmarshalStringSafe(b); err != nil {
		return err
	}
	return nil
}

type SshFxpWritePacket struct {
	ID     uint32
	Handle string
	Offset uint64
	Length uint32
	Data   []byte
}

func (p SshFxpWritePacket) id() uint32 { return p.ID }

func (p SshFxpWritePacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Handle) +
		8 + 4 + // uint64 + uint32
		len(p.Data)

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_WRITE)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Handle)
	b = marshalUint64(b, p.Offset)
	b = marshalUint32(b, p.Length)
	b = append(b, p.Data...)
	return b, nil
}

func (p *SshFxpWritePacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Handle, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Offset, b, err = unmarshalUint64Safe(b); err != nil {
		return err
	} else if p.Length, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if uint32(len(b)) < p.Length {
		return errShortPacket
	}

	p.Data = append([]byte{}, b[:p.Length]...)
	return nil
}

type SshFxpMkdirPacket struct {
	ID    uint32
	Path  string
	Flags uint32 // ignored
}

func (p SshFxpMkdirPacket) id() uint32 { return p.ID }

func (p SshFxpMkdirPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Path) +
		4 // uint32

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_MKDIR)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Path)
	b = marshalUint32(b, p.Flags)
	return b, nil
}

func (p *SshFxpMkdirPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Path, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Flags, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	return nil
}

type SshFxpSetstatPacket struct {
	ID    uint32
	Path  string
	Flags uint32
	Attrs interface{}
}

type SshFxpFsetstatPacket struct {
	ID     uint32
	Handle string
	Flags  uint32
	Attrs  interface{}
}

func (p SshFxpSetstatPacket) id() uint32  { return p.ID }
func (p SshFxpFsetstatPacket) id() uint32 { return p.ID }

func (p SshFxpSetstatPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Path) +
		4 // uint32 + uint64

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_SETSTAT)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Path)
	b = marshalUint32(b, p.Flags)
	b = marshal(b, p.Attrs)
	return b, nil
}

func (p SshFxpFsetstatPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		4 + len(p.Handle) +
		4 // uint32 + uint64

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_FSETSTAT)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Handle)
	b = marshalUint32(b, p.Flags)
	b = marshal(b, p.Attrs)
	return b, nil
}

func (p *SshFxpSetstatPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Path, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Flags, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	p.Attrs = b
	return nil
}

func (p *SshFxpFsetstatPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Handle, b, err = unmarshalStringSafe(b); err != nil {
		return err
	} else if p.Flags, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	}
	p.Attrs = b
	return nil
}

type SshFxpHandlePacket struct {
	ID     uint32
	Handle string
}

func (p SshFxpHandlePacket) MarshalBinary() ([]byte, error) {
	b := []byte{SSH_FXP_HANDLE}
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Handle)
	return b, nil
}

type SshFxpStatusPacket struct {
	ID uint32
	StatusError
}

func (p SshFxpStatusPacket) MarshalBinary() ([]byte, error) {
	b := []byte{SSH_FXP_STATUS}
	b = marshalUint32(b, p.ID)
	b = marshalStatus(b, p.StatusError)
	return b, nil
}

type SshFxpDataPacket struct {
	ID     uint32
	Length uint32
	Data   []byte
}

func (p SshFxpDataPacket) MarshalBinary() ([]byte, error) {
	b := []byte{SSH_FXP_DATA}
	b = marshalUint32(b, p.ID)
	b = marshalUint32(b, p.Length)
	b = append(b, p.Data[:p.Length]...)
	return b, nil
}

func (p *SshFxpDataPacket) UnmarshalBinary(b []byte) error {
	var err error
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.Length, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if uint32(len(b)) < p.Length {
		return errors.New("truncated packet")
	}

	p.Data = make([]byte, p.Length)
	copy(p.Data, b)
	return nil
}

type SshFxpStatvfsPacket struct {
	ID   uint32
	Path string
}

func (p SshFxpStatvfsPacket) id() uint32 { return p.ID }

func (p SshFxpStatvfsPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		len(p.Path) +
		len("statvfs@openssh.com")

	b := make([]byte, 0, l)
	b = append(b, SSH_FXP_EXTENDED)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, "statvfs@openssh.com")
	b = marshalString(b, p.Path)
	return b, nil
}

// A StatVFS contains statistics about a filesystem.
type StatVFS struct {
	ID      uint32
	Bsize   uint64 /* file system block size */
	Frsize  uint64 /* fundamental fs block size */
	Blocks  uint64 /* number of blocks (unit f_frsize) */
	Bfree   uint64 /* free blocks in file system */
	Bavail  uint64 /* free blocks for non-root */
	Files   uint64 /* total file inodes */
	Ffree   uint64 /* free file inodes */
	Favail  uint64 /* free file inodes for to non-root */
	Fsid    uint64 /* file system id */
	Flag    uint64 /* bit mask of f_flag values */
	Namemax uint64 /* maximum filename length */
}

// TotalSpace calculates the amount of total space in a filesystem.
func (p *StatVFS) TotalSpace() uint64 {
	return p.Frsize * p.Blocks
}

// FreeSpace calculates the amount of free space in a filesystem.
func (p *StatVFS) FreeSpace() uint64 {
	return p.Frsize * p.Bfree
}
