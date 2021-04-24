// +build darwin dragonfly freebsd !android,linux netbsd openbsd solaris aix
// +build cgo

package sftp

import (
	"os"
	"syscall"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

func attributesFromFileInfo(fi os.FileInfo) sshfx.Attributes {
	var attrs sshfx.Attributes

	switch sys := fi.Sys().(type) {
	case *syscall.Stat_t:
		attrs.SetSize(uint64(sys.Size))
		attrs.SetUIDGID(sys.Uid, sys.Gid)
		attrs.SetPermissions(sshfx.FileMode(sys.Mode))
		attrs.SetACModTime(uint32(sys.Atim.Sec), uint32(sys.Mtim.Sec))

	case *FileStat:
		attrs.SetSize(sys.Size)
		attrs.SetUIDGID(sys.UID, sys.GID)
		attrs.SetPermissions(sshfx.FileMode(sys.Mode))
		attrs.SetACModTime(sys.Atime, sys.Mtime)

	case *sshfx.Attributes:
		attrs.SetSize(sys.Size)
		attrs.SetUIDGID(sys.UID, sys.GID)
		attrs.SetPermissions(sys.Permissions)
		attrs.SetACModTime(sys.ATime, sys.MTime)

	default:
		attrs.SetSize(uint64(fi.Size()))
		attrs.SetPermissions(fromFileMode(fi.Mode()))

		mtime := uint32(fi.ModTime().Unix())
		attrs.SetACModTime(mtime, mtime)
	}

	return attrs
}
