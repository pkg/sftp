package sftp

// SSH_FXP_ATTRS support
// see http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02#section-5

import (
	"os"
	"time"
)

const (
	SSH_FILEXFER_ATTR_SIZE        = 0x00000001
	SSH_FILEXFER_ATTR_UIDGID      = 0x00000002
	SSH_FILEXFER_ATTR_PERMISSIONS = 0x00000004
	SSH_FILEXFER_ATTR_ACMODTIME   = 0x00000008
	SSH_FILEXFER_ATTR_EXTENDED    = 0x80000000
)

type attr struct {
	size  uint64
	mode  os.FileMode
	mtime time.Time
}

// Name returns the base name of the file.
func (a *attr) Name() string { return "" }

// Size returns the length in bytes for regular files; system-dependent for others.
func (a *attr) Size() int64 { return int64(a.size) }

// Mode returns file mode bits.
func (a *attr) Mode() os.FileMode { return a.mode }

// ModTime returns the last modification time of the file.
func (a *attr) ModTime() time.Time { return a.mtime }

// IsDir returns true if the file is a directory.
func (a *attr) IsDir() bool { return a.Mode().IsDir() }

func (a *attr) Sys() interface{} { return a }

func unmarshalAttrs(b []byte) (*attr, []byte) {
	flags, b := unmarshalUint32(b)
	var a attr
	if flags&SSH_FILEXFER_ATTR_SIZE == SSH_FILEXFER_ATTR_SIZE {
		a.size, b = unmarshalUint64(b)
	}
	if flags&SSH_FILEXFER_ATTR_UIDGID == SSH_FILEXFER_ATTR_UIDGID {
		_, b = unmarshalUint32(b) // discarded
	}
	if flags&SSH_FILEXFER_ATTR_UIDGID == SSH_FILEXFER_ATTR_UIDGID {
		_, b = unmarshalUint32(b) // discarded
	}
	if flags&SSH_FILEXFER_ATTR_PERMISSIONS == SSH_FILEXFER_ATTR_PERMISSIONS {
		var mode uint32
		mode, b = unmarshalUint32(b)
		a.mode = os.FileMode(mode)
	}
	if flags&SSH_FILEXFER_ATTR_ACMODTIME == SSH_FILEXFER_ATTR_ACMODTIME {
		var mtime uint32
		_, b = unmarshalUint32(b) // discarded
		mtime, b = unmarshalUint32(b)
		a.mtime = time.Unix(int64(mtime), 0)
	}
	if flags&SSH_FILEXFER_ATTR_EXTENDED == SSH_FILEXFER_ATTR_EXTENDED {
		var count uint32
		count, b = unmarshalUint32(b)
		for i := uint32(0); i < count; i++ {
			_, b = unmarshalString(b)
			_, b = unmarshalString(b)
		}
	}
	return &a, b
}
