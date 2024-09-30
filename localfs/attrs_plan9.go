package localfs

import (
	"io/fs"
	"syscall"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

func fileStatFromInfoOs(fi fs.FileInfo, attrs *sshfx.Attributes) {
	if dir, ok := fi.Sys().(*syscall.Dir); ok {
		attrs.ATime = dir.Atime
	}
}
