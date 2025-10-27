//go:build windows || android
// +build windows android

package sftp

import (
	"os"
)

func lsLinksUserGroup(fi os.FileInfo) (numLinks, uid, gid string) {
	return "?", "0", "0"
}
