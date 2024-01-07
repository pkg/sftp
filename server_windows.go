//go:build go1.18

package sftp

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"golang.org/x/sys/windows"
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

func bitsToDrives(bitmap uint32) []string {
	var drive rune = 'a'
	var drives []string

	for bitmap != 0 {
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
		return nil, fmt.Errorf("GetLogicalDrives: %w", err)
	}
	return bitsToDrives(mask), nil
}

type driveInfo struct {
	fs.FileInfo
	name string
}

func (i *driveInfo) Name() string {
	return i.name // since the Name() returned from a os.Stat("C:\\") is "\\"
}

type winRoot struct {
	dummyFile
	doneDirs int
}

func (f *winRoot) Readdir(n int) ([]os.FileInfo, error) {
	drives, err := getDrives()
	if err != nil {
		return nil, err
	}

	if f.doneDirs >= len(drives) {
		return nil, io.EOF
	}
	drives = drives[f.doneDirs:]

	var infos []os.FileInfo
	for i, drive := range drives {
		if i >= n {
			break
		}

		fi, err := os.Stat(drive)
		if err != nil {
			return nil, err
		}

		di := &driveInfo{
			FileInfo: fi,
			name:     drive,
		}
		infos = append(infos, di)
	}

	f.doneDirs += len(infos)
	return infos, nil
}

func openfile(path string, flag int, mode fs.FileMode) (file, error) {
	if path == "/" {
		return &winRoot{}, nil
	}
	return os.OpenFile(path, flag, mode)
}
