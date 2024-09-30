//go:build darwin || dragonfly
// +build darwin dragonfly

package statvfs

import (
	"syscall"
)

func statvfsNameMax(_ syscall.Statfs_t) uint64 {
	// darwin:    man 2 statfs shows: #define MAXPATHLEN 1024
	// dragonfly:     params.h shows: #define MAXPATHLEN MAX_PATH; syslimits.h shows: #define PATH_MAX 1024
	return 1024
}
