// +build aix darwin dragonfly freebsd !android,linux netbsd openbsd solaris

package sftp

import (
	"os"
	"syscall"
)

func lsLinksUIDGID(fi os.FileInfo) (numLinks uint64, uid, gid string) {
	numLinks = 1
	uid, gid = "0", "0"

	switch sys := fi.Sys().(type) {
	case *syscall.Stat_t:
		numLinks = uint64(sys.Nlink)
		uid = lsUsername(lsFormatID(sys.Uid))
		gid = lsGroupName(lsFormatID(sys.Gid))
	default:
	}

	return numLinks, uid, gid
}
