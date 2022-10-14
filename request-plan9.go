// +build plan9

package sftp

import (
	"path"
	"path/filepath"
	"syscall"
)

func fakeFileInfoSys() interface{} {
	return &syscall.Dir{}
}

func testOsSys(sys interface{}) error {
	return nil
}

func toLocalPath(workDir, p string) string {
	if workDir != "" {
		if !filepath.IsAbs(p) && !path.IsAbs(p) {
			// Ensure input is always in the same format.
			p = filepath.ToSlash(p)
			p = path.Join(workDir, p)
		}
	}

	lp := filepath.FromSlash(p)

	if path.IsAbs(p) {
		tmp := lp[1:]

		if filepath.IsAbs(tmp) {
			// If the FromSlash without any starting slashes is absolute,
			// then we have a filepath encoded with a prefix '/'.
			// e.g. "/#s/boot" to "#s/boot"
			return tmp
		}

	}

	return lp
}
