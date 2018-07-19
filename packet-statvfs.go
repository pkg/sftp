package sftp

import (
	"bytes"
	"encoding/binary"
)

type SSHFxpStatvfsPacket struct {
	ID   uint32
	Path string
}

func (p SSHFxpStatvfsPacket) Id() uint32 { return p.ID }

func (p SSHFxpStatvfsPacket) GetPath() string { return p.Path }

func (p SSHFxpStatvfsPacket) MarshalBinary() ([]byte, error) {
	l := 1 + 4 + // type(byte) + uint32
		len(p.Path) +
		len("statvfs@openssh.com")

	b := make([]byte, 0, l)
	b = append(b, ssh_FXP_EXTENDED)
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

func (p *StatVFS) Id() uint32 {
	return p.ID
}

// TotalSpace calculates the amount of total space in a filesystem.
func (p *StatVFS) TotalSpace() uint64 {
	return p.Frsize * p.Blocks
}

// FreeSpace calculates the amount of free space in a filesystem.
func (p *StatVFS) FreeSpace() uint64 {
	return p.Frsize * p.Bfree
}

// Convert to ssh_FXP_EXTENDED_REPLY packet binary format
func (p *StatVFS) MarshalBinary() ([]byte, error) {
	var buf bytes.Buffer
	buf.Write([]byte{ssh_FXP_EXTENDED_REPLY})
	err := binary.Write(&buf, binary.BigEndian, p)
	return buf.Bytes(), err
}
