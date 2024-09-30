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

type ServerHandler struct {
	// sftp.UnimplementedHandler

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

func (h *ServerHandler) Mkdir(_ context.Context, req *sshfx.MkdirPacket) error {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return err
	}

	perm, ok := req.Attrs.GetPermissions()
	perm = perm.Perm()
	if !ok {
		perm = 0755
	}

	return os.Mkdir(lpath, fs.FileMode(perm))
}

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

func (h *ServerHandler) SetStat(_ context.Context, req *sshfx.SetStatPacket) error {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return err
	}

	if size, ok := req.Attrs.GetSize(); ok {
		if err := os.Truncate(lpath, int64(size)); err != nil {
			return err
		}
	}

	if uid, gid, ok := req.Attrs.GetUIDGID(); ok {
		if err := os.Chown(lpath, int(uid), int(gid)); err != nil {
			return err
		}
	}

	if perm, ok := req.Attrs.GetPermissions(); ok {
		if err := os.Chmod(lpath, fs.FileMode(perm.Perm())); err != nil {
			return err
		}
	}

	if atime, mtime, ok := req.Attrs.GetACModTime(); ok {
		if err := os.Chtimes(lpath, time.Unix(int64(atime), 0), time.Unix(int64(mtime), 0)); err != nil {
			return err
		}
	}

	return nil
}

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
	attrs := &sshfx.Attributes{
		Flags: sshfx.AttrSize | sshfx.AttrACModTime | sshfx.AttrPermissions,

		Size:  uint64(fi.Size()),
		MTime: uint32(fi.ModTime().Unix()),

		Permissions: sshfx.FromGoFileMode(fi.Mode()),
	}

	fileStatFromInfoOs(fi, attrs)

	return attrs
}

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

func (h *ServerHandler) ReadLink(_ context.Context, req *sshfx.ReadLinkPacket) (string, error) {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return "", err
	}

	return os.Readlink(lpath)
}

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

	perm, ok := req.Attrs.GetPermissions()
	perm = perm.Perm()
	if !ok {
		perm = 0666
	}

	return h.openfile(lpath, osFlags, fs.FileMode(perm))
}

func (h *ServerHandler) OpenDir(_ context.Context, req *sshfx.OpenDirPacket) (sftp.DirHandler, error) {
	lpath, err := h.toLocalPath(req.Path)
	if err != nil {
		return nil, err
	}

	return h.openfile(lpath, os.O_RDONLY, 0)
}
