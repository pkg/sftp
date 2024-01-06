//go:build go1.18

package sftp

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"time"
)

func (s *Server) toLocalPath(p string) string {
	if s.workDir != "" && !path.IsAbs(p) {
		p = path.Join(s.workDir, p)
	}

	lp := filepath.FromSlash(p)

	if path.IsAbs(p) {
		tmp := lp
		for len(tmp) > 0 && tmp[0] == '\\' {
			tmp = tmp[1:]
		}

		if filepath.IsAbs(tmp) {
			// If the FromSlash without any starting slashes is absolute,
			// then we have a filepath encoded with a prefix '/'.
			// e.g. "/C:/Windows" to "C:\\Windows"
			return tmp
		}

		tmp += "\\"

		if filepath.IsAbs(tmp) {
			// If the FromSlash without any starting slashes but with extra end slash is absolute,
			// then we have a filepath encoded with a prefix '/' and a dropped '/' at the end.
			// e.g. "/C:" to "C:\\"
			return tmp
		}
	}

	return lp
}

var kernel32, _ = syscall.LoadLibrary("kernel32.dll")
var getLogicalDrivesHandle, _ = syscall.GetProcAddress(kernel32, "GetLogicalDrives")

func bitsToDrives(bitmap uint32) []string {
	var drive rune = 'A'
	var drives []string

	for bitmap != 0 {
		if bitmap&1 == 1 {
			drives = append(drives, string(drive))
		}
		drive++
		bitmap >>= 1
	}

	return drives
}

func getDrives() ([]string, error) {
	if ret, _, callErr := syscall.Syscall(uintptr(getLogicalDrivesHandle), 0, 0, 0, 0); callErr != 0 {
		return nil, fmt.Errorf("GetLogicalDrives: %w", callErr)
	} else {
		drives := bitsToDrives(uint32(ret))
		return drives, nil
	}
}

type dummyDriveStat struct {
	name string
}

func (s *dummyDriveStat) Name() string {
	return s.name
}
func (s *dummyDriveStat) Size() int64 {
	return 1024
}
func (s *dummyDriveStat) Mode() os.FileMode {
	return os.FileMode(0755)
}
func (s *dummyDriveStat) ModTime() time.Time {
	return time.Now()
}
func (s *dummyDriveStat) IsDir() bool {
	return true
}
func (s *dummyDriveStat) Sys() any {
	return nil
}

type WinRoot struct {
	dummyFile
}

func (f *WinRoot) Readdir(int) ([]os.FileInfo, error) {
	drives, err := getDrives()
	if err != nil {
		return nil, err
	}
	infos := []os.FileInfo{}
	for _, drive := range drives {
		infos = append(infos, &dummyDriveStat{drive})
	}
	return infos, nil
}

func openFileLike(path string, flag int, mode fs.FileMode) (FileLike, error) {
	if path == "/" {
		return &WinRoot{}, nil
	}
	return os.OpenFile(path, flag, mode)
}
