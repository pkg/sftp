//go:build aix || freebsd || darwin || dragonfly || openbsd || linux
// +build aix freebsd darwin dragonfly openbsd linux

package localfs

import (
	"context"

	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
	"github.com/pkg/sftp/v2/localfs/statvfs"
)

func (f *File) StatVFS() (*openssh.StatVFSExtendedReplyPacket, error) {
	return statvfs.StatVFS(f.filename)
}

func (s *ServerHandler) StatVFS(_ context.Context, req *openssh.StatVFSExtendedPacket) (*openssh.StatVFSExtendedReplyPacket, error) {
	return statvfs.StatVFS(req.Path)
}
