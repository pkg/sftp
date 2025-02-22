//go:build (!js && !wasip1 && !darwin && !freebsd && !netbsd && !dragonfly && !linux && !openbsd && !solaris && !aix && !zos && !plan9 && !windows) || android
// +build !js,!wasip1,!darwin,!freebsd,!netbsd,!dragonfly,!linux,!openbsd,!solaris,!aix,!zos,!plan9,!windows android

package localfs

import (
	"io/fs"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

func fileStatFromInfoOs(fi fs.FileInfo, attrs *sshfx.Attributes) {
	// todo
}
