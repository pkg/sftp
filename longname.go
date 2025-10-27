package sftp

import (
	"fmt"
	"io/fs"
	"time"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

// NameLookup defines an interface to lookup user names and group names in a portable manner.
type NameLookup interface {
	LookupUserName(string) string
	LookupGroupName(string) string
}

// FormatLongname formats the FileInfo as per `ls -l` style, which is in the 'longname' field of a SSH_FXP_NAME entry.
// This should be enough to look close to openssh for typical use cases.
func FormatLongname(fi fs.FileInfo, idLookup NameLookup) string {
	// example from openssh sftp server:
	// crw-rw-rw-    1 root     wheel           0 Jul 31 20:52 ttyvd
	// format:
	// {directory / char device / etc}{rwxrwxrwx}  {number of links} owner group size month day [time (this year) | year (otherwise)] name

	if fi == nil {
		return ""
	}

	//            rwxrwxrwx
	symPerms := "?---------"
	numLinks, uid, gid := lsLinksUserGroup(fi)

	switch sys := fi.Sys().(type) {
	case *sshfx.Attributes:
		symPerms = sys.GetPermissions().String()

		sysUID, sysGID := sys.GetUserGroup()
		uid = fmt.Sprint(sysUID)
		gid = fmt.Sprint(sysGID)

	default:
		symPerms = sshfx.FromGoFileMode(fi.Mode()).String()
	}

	if idLookup != nil {
		uid, gid = idLookup.LookupUserName(uid), idLookup.LookupGroupName(gid)
	}

	mtime := fi.ModTime()
	month := mtime.Format("Jan")
	day := mtime.Format("2")

	var yearOrTime string
	if mtime.Before(time.Now().AddDate(0, -6, 0)) {
		yearOrTime = mtime.Format("2006")
	} else {
		yearOrTime = mtime.Format("15:04")
	}

	return fmt.Sprintf("%s %4s %-8s %-8s %8d %s % 2s %5s %s", symPerms, numLinks, uid, gid, fi.Size(), month, day, yearOrTime, fi.Name())
}
