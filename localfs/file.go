package localfs

import (
	"cmp"
	"io/fs"
	"iter"
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
	lastErr error
	lastEnt *sshfx.NameEntry
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
// No error will be yielded until all available FileInfos have been yielded.
// Only one error will be yielded per invocation.
//
// We do not expose an iterator, because none has been standardized yet,
// and we do not want to accidentally implement an API inconsistent with future standards.
// However, for internal usage, we can separate the paginated Readdir code from the conversion to SFTP entries.
//
// Callers must guarantee synchronization by either holding the file lock, or holding an exclusive reference.
func (f *File) rangedir(grow func(int)) iter.Seq2[fs.FileInfo, error] {
	return func(yield func(fs.FileInfo, error) bool) {
		for {
			grow(len(f.entries))

			for i, entry := range f.entries {
				if !yield(entry, nil) {
					// This is a break condition.
					// We need to remove all entries that have been consumed,
					// and that includes the one we are currently on.
					f.entries = slices.Delete(f.entries, 0, i+1)
					return
				}
			}

			// We have consumed all of the saved entries, so we remove everything.
			f.entries = slices.Delete(f.entries, 0, len(f.entries))

			if f.lastErr != nil {
				yield(nil, f.lastErr)
				f.lastErr = nil
				return
			}

			// We cannot guarantee we only get entries, or an error, never both.
			// So we need to just save these, and loop.
			f.entries, f.lastErr = f.Readdir(128)
		}
	}
}

// ReadDir overrides the [os.File.ReadDir] receiver method
// by converting the slice of [fs.DirEntry] into into a slice of [sshfx.NameEntry].
func (f *File) ReadDir(maxDataLen uint32) (entries []*sshfx.NameEntry, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.lastEnt != nil {
		// Last ReadDir left an entry for us to include in this call.
		entries = append(entries, f.lastEnt)
		f.lastEnt = nil
	}

	grow := func(more int) {
		entries = slices.Grow(entries, more)
	}

	var size int
	for fi, err := range f.rangedir(grow) {
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

		size += entry.MarshalSize()

		if size > int(maxDataLen) {
			// This would exceed the packet data length,
			// so save this one for the next call,
			// and return.
			f.lastEnt = entry
			break
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// SetStat implements [sftp.SetStatFileHandler].
func (f *File) SetStat(attrs *sshfx.Attributes) (err error) {
	if len(attrs.Extended) > 0 {
		err = &sshfx.StatusPacket{
			StatusCode:   sshfx.StatusOpUnsupported,
			ErrorMessage: "unsupported fsetstat: extended atributes",
		}
	}

	if attrs.HasSize() {
		sz := attrs.GetSize()
		err = cmp.Or(f.Truncate(int64(sz)), err)
	}

	if attrs.HasPermissions() {
		perm := attrs.GetPermissions()
		err = cmp.Or(f.Chmod(fs.FileMode(perm.Perm())), err)
	}

	if attrs.HasACModTime() {
		atime, mtime := attrs.GetACModTime()
		err = cmp.Or(os.Chtimes(f.filename, time.Unix(int64(atime), 0), time.Unix(int64(mtime), 0)), err)
	}

	if attrs.HasUIDGID() {
		uid, gid := attrs.GetUIDGID()
		err = cmp.Or(f.Chown(int(uid), int(gid)), err)
	}

	return err
}
