// +build aix dragonfly !android,linux openbsd solaris

package sftp

import (
	"os"
	"syscall"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

func attributesFromFileInfo(fi os.FileInfo) sshfx.Attributes {
	if sys, ok := fi.Sys().(*syscall.Stat_t); ok {
		var attrs sshfx.Attributes

		attrs.SetSize(uint64(sys.Size))
		attrs.SetUIDGID(sys.Uid, sys.Gid)
		attrs.SetPermissions(sshfx.FileMode(sys.Mode))
		attrs.SetACModTime(uint32(sys.Atim.Sec), uint32(sys.Mtim.Sec))

		return attrs
	}

	return attributesFromGenericFileInfo(fi)
}
