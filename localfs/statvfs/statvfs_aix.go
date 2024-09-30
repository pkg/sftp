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

	return &openssh.StatVFSExtendedReplyPacket{
		BlockSize:    stat.Bsize,
		FragmentSize: stat.Fsize,
		Blocks:       stat.Blocks,
		BlocksFree:   stat.Bfree,
		BlocksAvail:  stat.Bavail,
		Files:        stat.Files,
		FilesFree:    stat.Ffree,
		FilesAvail:   stat.Ffree,       // no exact equivalent
		FilesystemID: stat.Fsid.Val[0], // We lose the top uint64!
		// HAVE_STRUCT_STATFS_F_FLAGS == false
		MaxNameLength: uint64(stat.Name_max),
	}, nil
}
