//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package localfs

import (
	"context"
	"io/fs"
	"os"
	"time"

	"golang.org/x/sys/unix"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
)

// LSetStat implements the "lsetstat@openssh.com" extension.
// It is the same as SetStat, except that it does not follow symbolic links.
func (h *ServerHandler) LSetStat(_ context.Context, req *openssh.LSetStatExtendedPacket) error {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return err
	}

	if req.Attrs.HasSize() {
		// Setting the size of a symbolic link is nonsensical.
		return &sshfx.StatusPacket{
			StatusCode:   sshfx.StatusBadMessage,
			ErrorMessage: "lsetstat: cannot set size",
		}
	}

	if req.Attrs.HasUserGroup() {
		uid, gid := req.Attrs.GetUserGroup()
		if err := os.Lchown(lpath, int(uid), int(gid)); err != nil {
			return err
		}
	}

	if req.Attrs.HasPermissions() {
		perms := req.Attrs.GetPermissions()
		if err := unix.Fchmodat(unix.AT_FDCWD, lpath, uint32(perms.Perm()), unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return &fs.PathError{Op: "lchmod", Path: lpath, Err: err}
		}
	}

	if req.Attrs.HasACModTime() {
		atime, mtime := req.Attrs.GetACModTime()
		ts := []unix.Timespec{
			unix.NsecToTimespec(time.Unix(int64(atime), 0).UnixNano()),
			unix.NsecToTimespec(time.Unix(int64(mtime), 0).UnixNano()),
		}
		if err := unix.UtimesNanoAt(unix.AT_FDCWD, lpath, ts, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return &fs.PathError{Op: "lchtimes", Path: lpath, Err: err}
		}
	}

	return nil
}
