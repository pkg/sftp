// +build plan9

package sftp

import (
	"fmt"
	"syscall"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

func testOsSys(sys interface{}) error {
	switch sys := sys.(type) {
	case *syscall.Dir:
		// sys.Uid and sys.Gid are strings instead of ints.

	case *sshfx.Attributes:
		uid, gid, ok := sys.GetUIDGID()
		if !ok {
			return nil
		}
		if uid != 65534 {
			return fmt.Errorf("UID failed to match: %d", uid)
		}
		if gid != 65534 {
			return fmt.Errorf("GID failed to match: %d", gid)
		}

	case *FileStat:
		if sys.UID != 65534 {
			return fmt.Errorf("UID failed to match: %d", sys.UID)
		}
		if sys.GID != 65534 {
			return fmt.Errorf("GID failed to match: %d", sys.GID)
		}

	default:
		return fmt.Errorf("unexpected FileInfo.Sys() type: %T", sys)
	}

	return nil
}
