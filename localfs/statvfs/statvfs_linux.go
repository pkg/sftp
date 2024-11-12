package statvfs

import (
	"syscall"

	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
)

const (
	mountFlagReadOnly = 0x01 // ST_RDONLY
	mountFlagNoSUID   = 0x02 // ST_NOSUID
)

// StatVFS converts the syscall.Statfs from the Linux syscall to OpenSSH StatVFS.
func StatVFS(name string) (*openssh.StatVFSExtendedReplyPacket, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(name, &stat); err != nil {
		return nil, err
	}

	var flags uint64
	if stat.Flags&mountFlagReadOnly != 0 {
		flags |= openssh.MountFlagsReadOnly
	}
	if stat.Flags&mountFlagNoSUID != 0 {
		flags |= openssh.MountFlagsNoSUID
	}

	return &openssh.StatVFSExtendedReplyPacket{
		BlockSize:     uint64(stat.Bsize),
		FragmentSize:  uint64(stat.Frsize),
		Blocks:        stat.Blocks,
		BlocksFree:    stat.Bfree,
		BlocksAvail:   stat.Bavail,
		Files:         stat.Files,
		FilesFree:     stat.Ffree,
		FilesAvail:    stat.Ffree, // no exact equivalent
		MountFlags:    flags,
		MaxNameLength: uint64(stat.Namelen),

		// OpenSSH FSID_TO_ULONG
		FilesystemID: uint64(uint64(stat.Fsid.X__val[0])<<32 | uint64(stat.Fsid.X__val[1])),
	}, nil
}
