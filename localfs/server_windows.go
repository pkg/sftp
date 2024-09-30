package localfs

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/pkg/sftp/v2"
	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

func toLocalPath(p string) (string, error) {
	if !path.IsAbs(p) {
		// Relative path: just convert from slashes.
		return filepath.FromSlash(p), nil
	}

	if p == "/" {
		// This returns a sorta technically valid filename.
		// This is a DOS device path specifier.
		// Paths that start with this _must_ then be followed by a volume specifier.
		// So, arguably, listing this path should enumerate drives.
		//
		// Since we cleaned the path in ServerHandler.toLocalPath,
		// we shouldn’t be able to return anything like this from any other code path.
		return `\\.\`, nil
	}

	// Convert the path and strip the leading '/'.
	lp := filepath.FromSlash(p[1:])

	if strings.HasPrefix(lp, `wsl.localhost\`) || strings.HasPrefix(lp, `wsl$\`) {
		// Exceptionally permit local Windows Subservice for Linux .
		return `\\` + lp, nil
	}

	if strings.HasPrefix(p, `\??\`) {
		// This is a native NT path
		return "", fs.ErrInvalid
	}

	if !filepath.IsAbs(lp) {
		// In ServerHandler.toLocalPath, we already removed all repeat slashes with path.Clean.
		// So, the only `filepath.IsAbs(lp) == true` paths are simple drive letter paths.
		//
		// This means, we avoid UNC paths, both device path specifiers, and \\wsl$\ paths.
		// Hopefully, this leads to a lower chance of accidentally exposing every UNC path the host can access.
		return "", fs.ErrInvalid
	}

	if len(lp) == 2 {
		// This could only be a simple drive letter.
		// We need to add a backslash at the end to be valid.
		return lp + `\`, nil
	}

	return lp, nil
}

func bitsToDrives(bitmap uint32) []string {
	var drive rune = 'a'
	var drives []string

	for bitmap != 0 && drive <= 'z' {
		if bitmap&1 == 1 {
			drives = append(drives, string(drive)+":")
		}

		drive++
		bitmap >>= 1
	}

	return drives
}

func getDrives() ([]string, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, &os.SyscallError{Syscall: "GetLogicalDrives", Err: err}
	}
	return bitsToDrives(mask), nil
}

type winRoot struct {
	filename string
	handle   string

	mtime uint32

	entries []*sshfx.NameEntry
}

func (h *ServerHandler) newWinRoot() (f *winRoot, myErr error) {
	drives, err := getDrives()
	if err != nil {
		return nil, err
	}

	var entries []*sshfx.NameEntry
	for _, drive := range drives {
		fi, err := os.Stat(drive + `\`)
		if err != nil {
			return nil, err
		}

		attrs := fileInfoToAttrs(fi)

		entry := &sshfx.NameEntry{
			Filename: drive,
			Attrs:    *attrs,
		}

		entry.Longname = sftp.FormatLongname(entry, h)

		entries = append(entries, entry)
	}

	return &winRoot{
		filename: "/",
		handle:   fmt.Sprint(h.handles.Add(1)),

		mtime: uint32(time.Now().Unix()),

		entries: entries,
	}, nil
}

func (f *winRoot) Close() error {
	for i := range f.entries {
		f.entries[i] = nil
	}
	f.entries = nil
	return nil
}

func (f *winRoot) Name() string {
	return "/"
}

func (f *winRoot) Handle() string {
	return f.handle
}

func (f *winRoot) Stat() (*sshfx.Attributes, error) {
	return &sshfx.Attributes{
		Flags: sshfx.AttrSize | sshfx.AttrPermissions | sshfx.AttrACModTime,

		MTime: f.mtime,
		ATime: f.mtime,

		Permissions: 0555,
	}, nil
}

func (f *winRoot) ReadDir(maxDataLen uint32) ([]*sshfx.NameEntry, error) {
	var ret []*sshfx.NameEntry

	size := 4

	for len(f.entries) > 0 {
		entry := f.entries[0]

		if size+entry.Len() > int(maxDataLen) {
			return ret, nil
		}

		f.entries[0] = nil // clear the pointer before shifting it out.
		f.entries = f.entries[1:]

		ret = append(ret, entry)
	}

	return ret, io.EOF
}

func (f *winRoot) SetStat(_ *sshfx.Attributes) error {
	return fs.ErrPermission
}

func (f *winRoot) ReadAt(_ []byte, _ int64) (int, error) {
	return 0, fs.ErrPermission
}

func (f *winRoot) WriteAt(_ []byte, _ int64) (int, error) {
	return 0, fs.ErrPermission
}

func (f *winRoot) Sync() error {
	return fs.ErrPermission
}

type fileDir interface {
	sftp.FileHandler
	sftp.DirHandler
}

func (h *ServerHandler) openfile(path string, flag int, mode fs.FileMode) (fileDir, error) {
	if path == `\\.\` {
		return h.newWinRoot()
	}

	f, err := os.OpenFile(path, flag, mode)
	if err != nil {
		return nil, err
	}

	return &File{
		filename: path,
		handle:   fmt.Sprint(h.handles.Add(1)),
		File:     f,
	}, nil
}
