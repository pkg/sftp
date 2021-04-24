// +build plan9
// +build cgo

package sftp

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

func formatID(id uint32) string {
	return strconv.FormatUint(uint64(id), 10)
}

func getUsername(uid string) string {
	u, err := user.LookupId(uid)
	if err != nil {
		return uid
	}

	return u.Username
}

func getGroupName(gid string) string {
	g, err := user.LookupGroupId(gid)
	if err != nil {
		return gid
	}

	return g.Name
}

// ls -l style output for a file, which is in the 'long output' section of a readdir response packet
// this is a very simple (lazy) implementation, just enough to look almost like openssh in a few basic cases
func runLs(dirname string, dirent os.FileInfo) string {
	// example from openssh sftp server:
	// crw-rw-rw-    1 root     wheel           0 Jul 31 20:52 ttyvd
	// format:
	// {directory / char device / etc}{rwxrwxrwx}  {number of links} owner group size month day [time (this year) | year (otherwise)] name

	symPerms := fromFileMode(dirent.Mode()).String()

	var numLinks uint64 = 1
	uid, gid := "0", "0"

	switch sys := dirent.Sys().(type) {
	case *syscall.Dir:
		uid = getUsername(sys.Uid)
		gid = getGroupName(sys.Gid)
	case *sshfx.Attributes:
		uid = formatID(sys.UID)
		gid = formatID(sys.GID)
	case *FileStat:
		uid = formatID(sys.UID)
		gid = formatID(sys.GID)
	}

	mtime := dirent.ModTime()
	date := mtime.Format("Jan 2")

	var yearOrTime string
	if mtime.Before(time.Now().AddDate(0, -6, 0)) {
		yearOrTime = mtime.Format("2006")
	} else {
		yearOrTime = mtime.Format("15:04")
	}

	return fmt.Sprintf("%s %4d %-8s %-8s %8d %s %5s %s", symPerms, numLinks, uid, gid, dirent.Size(), date, yearOrTime, dirent.Name())
}
