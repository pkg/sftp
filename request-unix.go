// +build !windows,!plan9

package sftp

import (
	"fmt"
	"syscall"
)

func testFileInfoSysOS(sys interface{}) error {
	switch sys := sys.(type) {
	case *syscall.Stat_t:
		if sys.Uid != 65534 {
			return fmt.Errorf("UID failed to match: %d", sys.Uid)
		}
		if sys.Gid != 65534 {
			return fmt.Errorf("GID failed to match: %d", sys.Gid)
		}

	default:
		return fmt.Errorf("unexpected FileInfo.Sys() type: %T", sys)
	}

	return nil
}
