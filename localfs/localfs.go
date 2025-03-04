package localfs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp/v2"
	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
)

// ServerHandler implements the sftp.ServerHandler interface using the local filesystem as the filesystem.
// NOTE: This is not normally a safe thing to expose.
type ServerHandler struct {
	sftp.UnimplementedServerHandler

	ReadOnly bool
	WorkDir  string

	handles atomic.Uint64
}

func (h *ServerHandler) toLocalPath(p string) (string, error) {
	if h.WorkDir != "" && !path.IsAbs(p) {
		p = path.Join(h.WorkDir, p)
	} else {
		// Ensure both paths are cleaning the path.
		// This has important reasons for Windows, but is a good idea in general.
		p = path.Clean(p)
	}

	if p == "" {
		return "", sshfx.StatusNoSuchFile
	}

	return toLocalPath(p)
}

// Mkdir implements [sftp.ServerHandler].
func (h *ServerHandler) Mkdir(_ context.Context, req *sshfx.MkdirPacket) error {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return err
	}

	var perms sshfx.FileMode = 0755
	if req.Attrs.HasPermissions() {
		perms = req.Attrs.GetPermissions().Perm()
	}

	return os.Mkdir(lpath, fs.FileMode(perms))
}

// Remove implements [sftp.ServerHandler].
func (h *ServerHandler) Remove(_ context.Context, req *sshfx.RemovePacket) error {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return err
	}

	fi, err := os.Stat(lpath)
	if err != nil {
		return err
	}

	if fi.IsDir() {
		return &fs.PathError{
			Op:   "remove",
			Path: lpath,
			Err:  fmt.Errorf("is a directory"),
		}
	}

	return os.Remove(lpath)
}

// Rename implements [sftp.ServerHandler].
func (h *ServerHandler) Rename(_ context.Context, req *sshfx.RenamePacket) error {
	from, err := h.toLocalPath(req.OldPath)
	if err != nil {
		return err
	}

	to, err := h.toLocalPath(req.NewPath)
	if err != nil {
		return err
	}

	if _, err := os.Stat(to); !errors.Is(err, fs.ErrNotExist) {
		if err == nil {
			return fs.ErrExist
		}

		return err
	}

	return os.Rename(from, to)
}

// POSIXRename implements [sftp.POSIXRenameServerHandler].
func (h *ServerHandler) POSIXRename(_ context.Context, req *openssh.POSIXRenameExtendedPacket) error {
	from, err := h.toLocalPath(req.OldPath)
	if err != nil {
		return err
	}

	to, err := h.toLocalPath(req.NewPath)
	if err != nil {
		return err
	}

	return posixRename(from, to)
}

// Rmdir implements [sftp.ServerHandler].
func (h *ServerHandler) Rmdir(_ context.Context, req *sshfx.RmdirPacket) error {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return err
	}

	fi, err := os.Stat(lpath)
	if err != nil {
		return err
	}

	if !fi.IsDir() {
		return &fs.PathError{
			Op:   "rmdir",
			Path: lpath,
			Err:  fmt.Errorf("not a directory"),
		}
	}

	return os.Remove(lpath)
}

// SetStat implements [sftp.ServerHandler].
func (h *ServerHandler) SetStat(_ context.Context, req *sshfx.SetStatPacket) error {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return err
	}

	if req.Attrs.HasSize() {
		sz := req.Attrs.GetSize()
		if err := os.Truncate(lpath, int64(sz)); err != nil {
			return err
		}
	}

	if req.Attrs.HasUIDGID() {
		uid, gid := req.Attrs.GetUIDGID()
		if err := os.Chown(lpath, int(uid), int(gid)); err != nil {
			return err
		}
	}

	if req.Attrs.HasPermissions() {
		perms := req.Attrs.GetPermissions()
		if err := os.Chmod(lpath, fs.FileMode(perms.Perm())); err != nil {
			return err
		}
	}

	if req.Attrs.HasACModTime() {
		atime, mtime := req.Attrs.GetACModTime()
		if err := os.Chtimes(lpath, time.Unix(int64(atime), 0), time.Unix(int64(mtime), 0)); err != nil {
			return err
		}
	}

	return nil
}

// Symlink implements [sftp.ServerHandler].
func (h *ServerHandler) Symlink(_ context.Context, req *sshfx.SymlinkPacket) error {
	target, err := h.toLocalPath(req.TargetPath)
	if err != nil {
		return err
	}

	link, err := h.toLocalPath(req.LinkPath)
	if err != nil {
		return err
	}

	return os.Symlink(target, link)
}

func fileInfoToAttrs(fi fs.FileInfo) *sshfx.Attributes {
	attrs := new(sshfx.Attributes)
	attrs.SetSize(uint64(fi.Size()))
	attrs.SetPermissions(sshfx.FromGoFileMode(fi.Mode()))

	mtime := uint32(fi.ModTime().Unix())
	attrs.SetACModTime(mtime, mtime)

	fileStatFromInfoOs(fi, attrs)

	return attrs
}

// LStat implements [sftp.ServerHandler].
func (h *ServerHandler) LStat(_ context.Context, req *sshfx.LStatPacket) (*sshfx.Attributes, error) {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return nil, err
	}

	fi, err := os.Lstat(lpath)
	if err != nil {
		return nil, err
	}

	return fileInfoToAttrs(fi), nil
}

// Stat implements [sftp.ServerHandler].
func (h *ServerHandler) Stat(_ context.Context, req *sshfx.StatPacket) (*sshfx.Attributes, error) {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return nil, err
	}

	fi, err := os.Stat(lpath)
	if err != nil {
		return nil, err
	}

	return fileInfoToAttrs(fi), nil
}

// ReadLink implements [sftp.ServerHandler].
func (h *ServerHandler) ReadLink(_ context.Context, req *sshfx.ReadLinkPacket) (string, error) {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return "", err
	}

	return os.Readlink(lpath)
}

// RealPath implements [sftp.ServerHandler].
func (h *ServerHandler) RealPath(_ context.Context, req *sshfx.RealPathPacket) (string, error) {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(lpath)
	if err != nil {
		return "", err
	}

	return path.Join("/", filepath.ToSlash(abs)), nil
}

// Open implements [sftp.ServerHandler].
func (h *ServerHandler) Open(_ context.Context, req *sshfx.OpenPacket) (sftp.FileHandler, error) {
	lpath, err := h.toLocalPath(req.Filename)
	if err != nil {
		return nil, err
	}

	var osFlags int

	switch {
	case req.PFlags&sshfx.FlagRead != 0:
		if req.PFlags&sshfx.FlagWrite != 0 && !h.ReadOnly {
			osFlags |= os.O_RDWR
		} else {
			osFlags |= os.O_RDONLY
		}

	case req.PFlags&sshfx.FlagWrite != 0:
		if h.ReadOnly {
			return nil, sshfx.StatusPermissionDenied
		}
		osFlags |= os.O_WRONLY

	default:
		return nil, fs.ErrInvalid
	}

	// Don't use O_APPEND flag as it conflicts with WriteAt.
	// The sshfx.FlagAppend is a no-op here as the client sends the offsets anyways.

	if req.PFlags&sshfx.FlagCreate != 0 {
		osFlags |= os.O_CREATE
	}
	if req.PFlags&sshfx.FlagTruncate != 0 {
		osFlags |= os.O_TRUNC
	}
	if req.PFlags&sshfx.FlagExclusive != 0 {
		osFlags |= os.O_EXCL
	}

	// Like OpenSSH, we only handle permissions here, and only when the file is being created.
	// Otherwise, the permissions are ignored.

	var perms sshfx.FileMode = 0666
	if req.Attrs.HasPermissions() {
		perms = req.Attrs.GetPermissions().Perm()
	}

	return h.openfile(lpath, osFlags, fs.FileMode(perms))
}

// OpenDir implements [sftp.ServerHandler].
func (h *ServerHandler) OpenDir(_ context.Context, req *sshfx.OpenDirPacket) (sftp.DirHandler, error) {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return nil, err
	}

	return h.openfile(lpath, os.O_RDONLY, 0)
}
