package statvfs

import (
	"syscall"

	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
)

func StatVFS(name string) (*openssh.StatVFSExtendedReplyPacket, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(name, &stat); err != nil {
		return nil, err
	}

	// OpenBSD cvt_statfs_to_statvfs
	return &openssh.StatVFSExtendedReplyPacket{
		BlockSize:    uint64(stat.F_iosize),
		FragmentSize: uint64(stat.F_bsize),

		Blocks:      stat.F_blocks,
		BlocksFree:  stat.F_bfree,
		BlocksAvail: uint64(max(0, stat.F_bavail)),

		Files:      stat.F_files,
		FilesFree:  stat.F_ffree,
		FilesAvail: uint64(max(0, stat.F_favail)),

		MountFlags:    statvfsFlags(uint64(stat.F_flags)),
		MaxNameLength: uint64(stat.F_namemax),

		// OpenSSH FSID_TO_ULONG
		FilesystemID: uint64(uint64(stat.F_fsid.Val[0])<<32 | uint64(stat.F_fsid.Val[1])),
	}, nil
}
