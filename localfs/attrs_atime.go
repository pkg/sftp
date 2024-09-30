//go:build js
// +build js

package localfs

import (
	"io/fs"
	"syscall"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

func fileStatFromInfoOs(fi fs.FileInfo, attrs *sshfx.Attributes) {
	if statt, ok := fi.Sys().(*syscall.Stat_t); ok {
		attrs.SetUIDGID(statt.Uid, statt.Gid)
		attrs.ATime = uint32(statt.Atime)
	}
}
