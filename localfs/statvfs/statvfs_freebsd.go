package statvfs

import (
	"syscall"
)

func statvfsNameMax(stat syscall.Statfs_t) uint64 {
	return uint64(stat.Namemax)
}
