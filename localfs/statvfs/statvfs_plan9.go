package statvfs

import (
	"syscall"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
)

// StatVFS stubs the OpenSSH StatVFS with an sshfx.StatusOpUnsupported Status.
func StatVFS(name string) (*openssh.StatVFSExtendedReplyPacket, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: syscall.EPLAN9.Error(),
	}
}
