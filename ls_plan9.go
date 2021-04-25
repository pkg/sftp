// +build plan9
// +build cgo

package sftp

import (
	"os"
	"syscall"
)

func lsLinksUIDGID(fi os.FileInfo) (numLinks uint64, uid, gid string) {
	numLinks = 1
	uid, gid = "0", "0"

	switch sys := fi.Sys().(type) {
	case *syscall.Dir:
		uid = lsUsername(sys.Uid)
		gid = lsGroupName(sys.Gid)
	}

	return numLinks, uid, gid
}
