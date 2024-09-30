//go:build windows || android
// +build windows android

package sftp

import (
	"os"
)

func lsLinksUIDGID(fi os.FileInfo) (numLinks, uid, gid string) {
	return "?", "0", "0"
}
