//go:build plan9
// +build plan9

package sftp

import (
	"os"
	"syscall"
)

func lsLinksUIDGID(fi os.FileInfo) (numLinks, uid, gid string) {
	numLinks, uid, gid = "1", "0", "0"

	switch sys := fi.Sys().(type) {
	case *syscall.Dir:
		uid = sys.Uid
		gid = sys.Gid
	}

	return
}
