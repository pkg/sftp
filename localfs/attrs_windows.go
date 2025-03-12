package localfs

import (
	"io/fs"
	"syscall"
	"time"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

func fileStatFromInfoOs(fi fs.FileInfo, attrs *sshfx.Attributes) {
	if fad, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		atime := uint32(time.Duration(fad.LastAccessTime.Nanoseconds()).Seconds())
		mtime := uint32(time.Duration(fad.LastWriteTime.Nanoseconds()).Seconds())
		attrs.SetACModTime(atime, mtime)
	}
}
