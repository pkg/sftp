//go:build aix || freebsd || darwin || dragonfly || openbsd || linux
// +build aix freebsd darwin dragonfly openbsd linux

package localfs

import (
	"github.com/pkg/sftp/v2"
)

var _ sftp.StatVFSFileHandler = &File{}

var _ sftp.StatVFSServerHandler = &ServerHandler{}
