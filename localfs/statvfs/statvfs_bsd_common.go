//go:build darwin || dragonfly || freebsd || openbsd
// +build darwin dragonfly freebsd openbsd

package statvfs

import (
	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
)

const (
	mountFlagReadOnly = 0x01 // MNT_RDONLY
	mountFlagNoSUID   = 0x08 // MNT_NOSUID
)

func statvfsFlags(sfsFlags uint64) (vfsFlags uint64) {
	if sfsFlags&mountFlagReadOnly != 0 {
		vfsFlags |= openssh.MountFlagsReadOnly
	}
	if sfsFlags&mountFlagNoSUID != 0 {
		vfsFlags |= openssh.MountFlagsNoSUID
	}
	return
}
