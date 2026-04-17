//go:build dragonfly || (!android && linux) || openbsd || solaris || aix || zos
// +build dragonfly !android,linux openbsd solaris aix zos

package localfs

import (
	"io/fs"
	"syscall"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

func fileStatFromInfoOs(fi fs.FileInfo, attrs *sshfx.Attributes) {
	if statt, ok := fi.Sys().(*syscall.Stat_t); ok {
		attrs.SetUserGroup(statt.Uid, statt.Gid)
		attrs.SetACModTime(uint32(statt.Atim.Sec), uint32(statt.Mtim.Sec))
	}
}
