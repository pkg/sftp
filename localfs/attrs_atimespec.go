//go:build darwin || freebsd || netbsd
// +build darwin freebsd netbsd

package localfs

import (
	"io/fs"
	"syscall"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

func fileStatFromInfoOs(fi fs.FileInfo, attrs *sshfx.Attributes) {
	if statt, ok := fi.Sys().(*syscall.Stat_t); ok {
		attrs.SetUserGroup(statt.Uid, statt.Gid)
		attrs.SetACModTime(uint32(statt.Atimespec.Sec), uint32(statt.Mtimespec.Sec))
	}
}
