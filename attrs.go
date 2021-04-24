package sftp

// ssh_FXP_ATTRS support
// see http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-5

import (
	"os"
	"time"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

// fileInfo is an artificial type designed to satisfy os.FileInfo.
type fileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	mtime time.Time
	sys   interface{}
}

// Name returns the base name of the file.
func (fi *fileInfo) Name() string { return fi.name }

// Size returns the length in bytes for regular files; system-dependent for others.
func (fi *fileInfo) Size() int64 { return fi.size }

// Mode returns file mode bits.
func (fi *fileInfo) Mode() os.FileMode { return fi.mode }

// ModTime returns the last modification time of the file.
func (fi *fileInfo) ModTime() time.Time { return fi.mtime }

// IsDir returns true if the file is a directory.
func (fi *fileInfo) IsDir() bool { return fi.Mode().IsDir() }

func (fi *fileInfo) Sys() interface{} { return fi.sys }

func fileInfoFromAttributes(name string, attrs sshfx.Attributes) os.FileInfo {
	return &fileInfo{
		name:  name,
		size:  int64(attrs.Size),
		mode:  toFileMode(attrs.Permissions),
		mtime: time.Unix(int64(attrs.MTime), 0),
		sys:   &attrs,
	}
}

// FileStat holds the original unmarshalled values from a call to READDIR or
// *STAT. It is exported for the purposes of accessing the raw values via
// os.FileInfo.Sys(). It is also used server side to store the unmarshalled
// values for SetStat.
type FileStat struct {
	Size     uint64
	Mode     uint32
	Mtime    uint32
	Atime    uint32
	UID      uint32
	GID      uint32
	Extended []StatExtended
}

// StatExtended contains additional, extended information for a FileStat.
type StatExtended struct {
	ExtType string
	ExtData string
}

func (stat *FileStat) toAttributes(flags uint32) sshfx.Attributes {
	attrs := sshfx.Attributes{
		Flags:       flags,
		Size:        stat.Size,
		UID:         stat.UID,
		GID:         stat.GID,
		Permissions: sshfx.FileMode(stat.Mode),
		ATime:       stat.Atime,
		MTime:       stat.Mtime,
	}

	if len(stat.Extended) > 0 {
		attrs.ExtendedAttributes = make([]sshfx.ExtendedAttribute, len(stat.Extended))

		for i, ext := range stat.Extended {
			attrs.ExtendedAttributes[i] = sshfx.ExtendedAttribute{
				Type: ext.ExtType,
				Data: ext.ExtData,
			}
		}
	}

	return attrs
}

func fromAttributes(attrs sshfx.Attributes) FileStat {
	stat := FileStat{
		Size:  attrs.Size,
		UID:   attrs.UID,
		GID:   attrs.GID,
		Mode:  uint32(attrs.Permissions),
		Atime: attrs.ATime,
		Mtime: attrs.MTime,
	}

	if len(attrs.ExtendedAttributes) > 0 {
		stat.Extended = make([]StatExtended, len(attrs.ExtendedAttributes))

		for i, ext := range attrs.ExtendedAttributes {
			stat.Extended[i] = StatExtended{
				ExtType: ext.Type,
				ExtData: ext.Data,
			}
		}
	}

	return stat
}
