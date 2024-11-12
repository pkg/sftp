//go:build darwin || dragonfly || freebsd
// +build darwin dragonfly freebsd

package statvfs

import (
	"syscall"

	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
)

// StatVFS converts the syscall.Statfs from the common FreeBSD syscall to OpenSSH StatVFS.
func StatVFS(name string) (*openssh.StatVFSExtendedReplyPacket, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(name, &stat); err != nil {
		return nil, err
	}

	// FreeBSD sfs2vfs
	return &openssh.StatVFSExtendedReplyPacket{
		BlockSize:    uint64(stat.Iosize),
		FragmentSize: uint64(stat.Bsize),

		Blocks:      uint64(stat.Blocks),
		BlocksFree:  uint64(stat.Bfree),
		BlocksAvail: uint64(stat.Bavail),

		Files:      uint64(stat.Files),
		FilesFree:  uint64(stat.Ffree),
		FilesAvail: uint64(stat.Ffree), // no exact equivalent

		MountFlags:    statvfsFlags(uint64(stat.Flags)),
		MaxNameLength: statvfsNameMax(stat),

		// OpenSSH FSID_TO_ULONG
		FilesystemID: uint64(uint64(stat.Fsid.Val[0])<<32 | uint64(stat.Fsid.Val[1])),
	}, nil
}
