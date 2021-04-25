// +build plan9

package sftp

import (
	"fmt"
	"syscall"
)

func testFileInfoSysOS(sys interface{}) error {
	switch sys := sys.(type) {
	case *syscall.Dir:
		// sys.Uid and sys.Gid are strings instead of ints.
	default:
		return fmt.Errorf("unexpected FileInfo.Sys() type: %T", sys)
	}

	return nil
}
