// +build !windows,!plan9

package sftp

import (
	"errors"
	"path"
	"syscall"
)

func fakeFileInfoSys() interface{} {
	return &syscall.Stat_t{Uid: 65534, Gid: 65534}
}

func testOsSys(sys interface{}) error {
	fstat := sys.(*FileStat)
	if fstat.UID != uint32(65534) {
		return errors.New("Uid failed to match")
	}
	if fstat.GID != uint32(65534) {
		return errors.New("Gid failed to match")
	}
	return nil
}

func toLocalPath(workDir, p string) string {
	if workDir != "" && !path.IsAbs(p) {
		p = path.Join(workDir, p)
	}

	return p
}
