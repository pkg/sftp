package sftp

import (
	"fmt"
	"syscall"
)

func testFileInfoSysOS(sys interface{}) error {
	switch sys := sys.(type) {
	case *syscall.Win32FileAttributeData:
	default:
		return fmt.Errorf("unexpected FileInfo.Sys() type: %T", sys)
	}

	return nil
}
