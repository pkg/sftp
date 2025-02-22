//go:build aix || darwin || dragonfly || freebsd || (!android && linux) || netbsd || openbsd || solaris || js || wasip1 || zos
// +build aix darwin dragonfly freebsd !android,linux netbsd openbsd solaris js wasip1 zos

package sftp

import (
	"fmt"
	"os"
	"syscall"
)

func lsLinksUIDGID(fi os.FileInfo) (numLinks, uid, gid string) {
	numLinks, uid, gid = "1", "0", "0"

	switch sys := fi.Sys().(type) {
	case *syscall.Stat_t:
		numLinks = fmt.Sprint(sys.Nlink)
		uid = fmt.Sprint(sys.Uid)
		gid = fmt.Sprint(sys.Gid)
	}

	return
}
