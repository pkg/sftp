//go:build !aix && !darwin && !dragonfly && !freebsd && !openbsd && !linux && !plan9
// +build !aix,!darwin,!dragonfly,!freebsd,!openbsd,!linux,!plan9

package statvfs

import (
	"runtime"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
)

func StatVFS(name string) (*openssh.StatVFSExtendedReplyPacket, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: "not supported by " + runtime.GOOS,
	}
}
