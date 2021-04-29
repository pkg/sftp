package sftp

import (
	"syscall"
)

func statvfsFromStatfst(stat *syscall.Statfs_t) (*StatVFS, error) {
	return &StatVFS{
		Bsize:  uint64(stat.Bsize),
		Frsize: uint64(stat.Bsize), // fragment size is a linux thing; use block size here
		Blocks: stat.Blocks,
		Bfree:  stat.Bfree,
		Bavail: stat.Bavail,
		Files:  stat.Files,
		Ffree:  stat.Ffree,
		Favail: stat.Ffree,                                              // not sure how to calculate
		Fsid:   uint64(stat.Fsid.Val[1])<<32 | uint64(stat.Fsid.Val[0]), // endianness?
		Flag:   uint64(stat.Flags),                                      // assuming POSIX?

		// statvfs.c: statvfs.namemax = NAME_MAX
		// syslimits.h: #define NAME_MAX 255
		Namemax: 255,
	}, nil
}
