package localfs

import (
	"cmp"
	"io/fs"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/pkg/sftp/v2"
	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

// File wraps an [os.File] to provide the additional operations necessary to implement [sftp.FileHandler].
type File struct {
	*os.File

	filename string
	handle   string
	idLookup sftp.NameLookup

	mu      sync.Mutex
	dirErr  error
	entries []fs.FileInfo
}

// Handle returns the SFTP handle associated with the file.
func (f *File) Handle() string {
	return f.handle
}

// Stat overrides the [os.File.Stat] receiver method
// by converting the [fs.FileInfo] into a [sshfx.Attributes].
func (f *File) Stat() (*sshfx.Attributes, error) {
	fi, err := f.File.Stat()
	if err != nil {
		return nil, err
	}

	return fileInfoToAttrs(fi), nil
}

// rangedir returns an iterator over the directory entries of the directory.
// It will only ever yield either a [fs.FileInfo] or an error, never both.
// No error will be yielded until all available FileInfos have been yielded,
// and thereafter the same error will be yielded indefinitely,
// however only one error will be yielded per invocation.
// If yield returns false, then the directory entry is considered unconsumed,
// and will be the first yield at the next call to rangedir.
//
// We do not expose an iterator, because none has been standardized yet,
// and we do not want to accidentally implement an API inconsistent with future standards.
// However, for internal usage, we can separate the paginated Readdir code from the conversion to SFTP entries.
//
// Callers must guarantee synchronization by either holding the file lock, or holding an exclusive reference.
func (f *File) rangedir(yield func(fs.FileInfo, error) bool) {
	for {
		for i, entry := range f.entries {
			if !yield(entry, nil) {
				// This is break condition.
				// As per our semantics, this means this entry has not been consumed.
				// So we remove only the entries ahead of this one.
				f.entries = slices.Delete(f.entries, 0, i)
				return
			}
		}

		// We have consumed all of the saved entries, so we remove everything.
		f.entries = slices.Delete(f.entries, 0, len(f.entries))

		if f.dirErr != nil {
			// No need to try acquiring more entries,
			// we’re already in the error state.
			yield(nil, f.dirErr)
			return
		}

		ents, err := f.Readdir(128)
		if err != nil {
			f.dirErr = err
		}

		f.entries = ents
	}
}

// ReadDir overrides the [os.File.ReadDir] receiver method
// by converting the slice of [fs.DirEntry] into into a slice of [sshfx.NameEntry].
func (f *File) ReadDir(maxDataLen uint32) (entries []*sshfx.NameEntry, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var size int
	for fi, err := range f.rangedir {
		if err != nil {
			if len(entries) != 0 {
				return entries, nil
			}

			return nil, err
		}

		attrs := fileInfoToAttrs(fi)

		entry := &sshfx.NameEntry{
			Filename: fi.Name(),
			Longname: sftp.FormatLongname(fi, f.idLookup),
			Attrs:    *attrs,
		}

		size += entry.Len()

		if size > int(maxDataLen) {
			// rangedir will take care of starting the next range with this entry.
			break
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// SetStat implements [sftp.SetStatFileHandler].
func (f *File) SetStat(attrs *sshfx.Attributes) (err error) {
	if size, ok := attrs.GetSize(); ok {
		err = cmp.Or(err, f.Truncate(int64(size)))
	}

	if perm, ok := attrs.GetPermissions(); ok {
		err = cmp.Or(err, f.Chmod(fs.FileMode(perm.Perm())))
	}

	if uid, gid, ok := attrs.GetUIDGID(); ok {
		err = cmp.Or(err, f.Chown(int(uid), int(gid)))
	}

	if atime, mtime, ok := attrs.GetACModTime(); ok {
		err = cmp.Or(err, os.Chtimes(f.filename, time.Unix(int64(atime), 0), time.Unix(int64(mtime), 0)))
	}

	return err
}
