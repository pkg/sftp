package localfs

import (
	"io/fs"
	"os"
	"time"

	"github.com/pkg/sftp/v2"
	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

type File struct {
	*os.File

	filename string
	handle   string
	idLookup sftp.NameLookup

	entries []*sshfx.NameEntry
}

func (f *File) Handle() string {
	return f.handle
}

func (f *File) Stat() (*sshfx.Attributes, error) {
	fi, err := f.File.Stat()
	if err != nil {
		return nil, err
	}

	return fileInfoToAttrs(fi), nil
}

func (f *File) ReadDir(maxDataLen uint32) ([]*sshfx.NameEntry, error) {
	var size int
	var ret []*sshfx.NameEntry
	for {
		for len(f.entries) > 0 {
			entry := f.entries[0]
			entryLen := entry.Len()

			if size+entryLen > int(maxDataLen) {
				// We would exceed the maxDataLen,
				// so keep the current top entry,
				// and return this partial response.
				return ret, nil
			}

			size += entryLen // accumulate size.

			f.entries[0] = nil // clear the pointer before shifting it out.
			f.entries = f.entries[1:]

			ret = append(ret, entry)
		}

		ents, err := f.Readdir(128)
		if err != nil && len(ents) == 0 {
			return ret, err
		}

		f.entries = make([]*sshfx.NameEntry, 0, len(ents))

		for _, fi := range ents {
			attrs := fileInfoToAttrs(fi)

			f.entries = append(f.entries, &sshfx.NameEntry{
				Filename: fi.Name(),
				Longname: sftp.FormatLongname(fi, f.idLookup),
				Attrs:    *attrs,
			})
		}
	}
}

func (f *File) SetStat(attrs *sshfx.Attributes) (err error) {
	if size, ok := attrs.GetSize(); ok {
		if err1 := f.Truncate(int64(size)); err == nil {
			err = err1
		}
	}

	if perm, ok := attrs.GetPermissions(); ok {
		if err1 := f.Chmod(fs.FileMode(perm.Perm())); err == nil {
			err = err1
		}
	}

	if uid, gid, ok := attrs.GetUIDGID(); ok {
		if err1 := f.Chown(int(uid), int(gid)); err == nil {
			err = err1
		}
	}

	if atime, mtime, ok := attrs.GetACModTime(); ok {
		if err1 := os.Chtimes(f.filename, time.Unix(int64(atime), 0), time.Unix(int64(mtime), 0)); err == nil {
			err = err1
		}
	}

	return err
}
