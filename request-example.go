package sftp

// This serves as an example of how to implement the request server handler as
// well as a dummy backend for testing. It implements an in-memory backend that
// works as a very simple filesystem with simple flat key-value lookup system.

import (
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxSymlinkFollows = 5

var errTooManySymlinks = errors.New("too many symbolic links")

// InMemHandler returns a Hanlders object with the test handlers.
func InMemHandler() Handlers {
	root := &root{
		rootFile: &memFile{name: "/", modtime: time.Now(), isdir: true},
		files:    make(map[string]*memFile),
	}
	return Handlers{root, root, root, root}
}

// Example Handlers
func (fs *root) Fileread(r *Request) (io.ReaderAt, error) {
	flags := r.Pflags()
	if !flags.Read {
		// sanity check
		return nil, os.ErrInvalid
	}

	// Needs to be readable by the user.
	return fs.openFileModeCheck(r, 0o0400)
}

func (fs *root) Filewrite(r *Request) (io.WriterAt, error) {
	flags := r.Pflags()
	if !flags.Write {
		// sanity check
		return nil, os.ErrInvalid
	}

	// Needs to be writable by the user.
	return fs.openFileModeCheck(r, 0o0200)
}

func (fs *root) OpenFile(r *Request) (WriterAtReaderAt, error) {
	// Needs to be readable and writable by the user.
	return fs.openFileModeCheck(r, 0o0200|0o0400)
}

func (fs *root) openFileModeCheck(r *Request, mode uint32) (WriterAtReaderAt, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	f, err := fs.openfile(r.Filepath, r.Flags)
	if err != nil {
		return nil, err
	}

	if f.mode&mode != mode {
		return nil, os.ErrPermission
	}

	return f, nil
}

func (fs *root) putfile(pathname string, file *memFile) error {
	pathname, err := fs.canonName(pathname)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(pathname, "/") {
		return os.ErrInvalid
	}

	if _, err := fs.lfetch(pathname); !errors.Is(err, os.ErrNotExist) {
		return os.ErrExist
	}

	file.name = pathname
	fs.files[pathname] = file

	return nil
}

func (fs *root) openfile(pathname string, flags uint32) (*memFile, error) {
	pflags := newFileOpenFlags(flags)

	file, err := fs.fetch(pathname)
	if err == os.ErrNotExist {
		if !pflags.Creat {
			return nil, os.ErrNotExist
		}

		var count int
		// You can create files through dangling symlinks.
		link, err := fs.lfetch(pathname)
		for err == nil && link.symlink != "" {
			if pflags.Excl {
				// unless you also passed in O_EXCL
				return nil, os.ErrInvalid
			}

			if count++; count > maxSymlinkFollows {
				return nil, errTooManySymlinks
			}

			pathname = link.symlink
			link, err = fs.lfetch(pathname)
		}

		// The mode is hard coded because the sftp protocol does not specify a
		// mode at file open time.
		file := &memFile{
			modtime: time.Now(),
			mode:    0644,
		}

		if err := fs.putfile(pathname, file); err != nil {
			return nil, err
		}

		return file, nil
	}

	if err != nil {
		return nil, err
	}

	if pflags.Creat && pflags.Excl {
		return nil, os.ErrExist
	}

	if file.IsDir() {
		return nil, os.ErrInvalid
	}

	if pflags.Trunc {
		if err := file.Truncate(0); err != nil {
			return nil, err
		}
	}

	return file, nil
}

func (fs *root) Filecmd(r *Request) error {
	if fs.mockErr != nil {
		return fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	switch r.Method {
	case "Setstat":
		// We explicitly do not support changing the mode of symlinks.
		// Nor do we want to change the mode of whatever the symlink is pointed
		// at.
		link, err := fs.Readlink(r.Filepath)
		if link != "" || err == nil || !errors.Is(err, os.ErrInvalid) {
			return err
		}

		// Note: fs.openfile does not support opening a directory.
		// So there is currently no way to set the mode of a directory.
		// That's a good thing, because it means that we don't have to do
		// permissions checks on parent directories.
		flags := r.AttrFlags()
		attrs := r.Attributes()
		file, err := fs.openfile(r.Filepath, sshFxfWrite)
		if err != nil {
			return err
		}

		if flags.Size {
			if err = file.Truncate(int64(attrs.Size)); err != nil {
				return err
			}
		}
		if flags.Permissions {
			const mask = uint32(os.ModePerm | s_ISUID | s_ISGID | s_ISVTX)
			file.mode = (file.mode &^ mask) | (attrs.Mode & mask)
		}
		// We only have mtime, not atime.
		if flags.Acmodtime {
			file.modtime = time.Unix(int64(attrs.Mtime), 0)
		}
		return nil

	case "Rename":
		// SFTP-v2: "It is an error if there already exists a file with the name specified by newpath."
		// This varies from the POSIX specification, which allows limited replacement of target files.
		if fs.exists(r.Target) {
			return os.ErrExist
		}

		return fs.rename(r.Filepath, r.Target)

	case "Rmdir":
		return fs.rmdir(r.Filepath)

	case "Remove":
		// IEEE 1003.1 remove explicitly can unlink files and remove empty directories.
		// We use instead here the semantics of unlink, which is allowed to be restricted against directories.
		return fs.unlink(r.Filepath)

	case "Mkdir":
		return fs.mkdir(r.Filepath)

	case "Link":
		return fs.link(r.Filepath, r.Target)

	case "Symlink":
		// NOTE: r.Filepath is the target, and r.Target is the linkpath.
		return fs.symlink(r.Filepath, r.Target)
	}

	return errors.New("unsupported")
}

func (fs *root) rename(oldpath, newpath string) error {
	file, err := fs.lfetch(oldpath)
	if err != nil {
		return err
	}

	newpath, err = fs.canonName(newpath)
	if err != nil {
		return err
	}

	if !strings.HasPrefix(newpath, "/") {
		return os.ErrInvalid
	}

	target, err := fs.lfetch(newpath)
	if !errors.Is(err, os.ErrNotExist) {
		if target == file {
			// IEEE 1003.1: if oldpath and newpath are the same directory entry,
			// then return no error, and perform no further action.
			return nil
		}

		switch {
		case file.IsDir():
			// IEEE 1003.1: if oldpath is a directory, and newpath exists,
			// then newpath must be a directory, and empty.
			// It is to be removed prior to rename.
			if err := fs.rmdir(newpath); err != nil {
				return err
			}

		case target.IsDir():
			// IEEE 1003.1: if oldpath is not a directory, and newpath exists,
			// then newpath may not be a directory.
			return syscall.EISDIR
		}
	}

	fs.files[newpath] = file

	if file.IsDir() {
		dirprefix := file.name + "/"

		for name, file := range fs.files {
			if strings.HasPrefix(name, dirprefix) {
				newname := path.Join(newpath, strings.TrimPrefix(name, dirprefix))

				fs.files[newname] = file
				file.name = newname
				delete(fs.files, name)
			}
		}
	}

	file.name = newpath
	delete(fs.files, oldpath)

	return nil
}

func (fs *root) PosixRename(r *Request) error {
	if fs.mockErr != nil {
		return fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	return fs.rename(r.Filepath, r.Target)
}

func (fs *root) StatVFS(r *Request) (*StatVFS, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}

	return getStatVFSForPath(r.Filepath)
}

func (fs *root) mkdir(pathname string) error {
	dir := &memFile{
		modtime: time.Now(),
		isdir:   true,
	}

	return fs.putfile(pathname, dir)
}

func (fs *root) rmdir(pathname string) error {
	// IEEE 1003.1: If pathname is a symlink, then rmdir should fail with ENOTDIR.
	dir, err := fs.lfetch(pathname)
	if err != nil {
		return err
	}

	if !dir.IsDir() {
		return syscall.ENOTDIR
	}

	// use the dir‘s internal name not the pathname we passed in.
	// the dir.name is always the canonical name of a directory.
	pathname = dir.name

	for name := range fs.files {
		if path.Dir(name) == pathname {
			return errors.New("directory not empty")
		}
	}

	delete(fs.files, pathname)

	return nil
}

func (fs *root) link(oldpath, newpath string) error {
	file, err := fs.lfetch(oldpath)
	if err != nil {
		return err
	}

	if file.IsDir() {
		return errors.New("hard link not allowed for directory")
	}

	return fs.putfile(newpath, file)
}

// symlink() creates a symbolic link named `linkpath` which contains the string `target`.
// NOTE! This would be called with `symlink(req.Filepath, req.Target)` due to different semantics.
func (fs *root) symlink(target, linkpath string) error {
	link := &memFile{
		modtime: time.Now(),
		symlink: target,
	}

	return fs.putfile(linkpath, link)
}

func (fs *root) unlink(pathname string) error {
	// does not follow symlinks!
	file, err := fs.lfetch(pathname)
	if err != nil {
		return err
	}

	if file.IsDir() {
		// IEEE 1003.1: implementations may opt out of allowing the unlinking of directories.
		// SFTP-v2: SSH_FXP_REMOVE may not remove directories.
		return os.ErrInvalid
	}

	// DO NOT use the file’s internal name.
	// because of hard-links files cannot have a single canonical name.
	delete(fs.files, pathname)

	return nil
}

type listerat []os.FileInfo

// Modeled after strings.Reader's ReadAt() implementation
func (f listerat) ListAt(ls []os.FileInfo, offset int64) (int, error) {
	var n int
	if offset >= int64(len(f)) {
		return 0, io.EOF
	}
	n = copy(ls, f[offset:])
	if n < len(ls) {
		return n, io.EOF
	}
	return n, nil
}

func (fs *root) Filelist(r *Request) (ListerAt, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	switch r.Method {
	case "List":
		files, err := fs.readdir(r.Filepath)
		if err != nil {
			return nil, err
		}
		return listerat(files), nil

	case "Stat":
		file, err := fs.fetch(r.Filepath)
		if err != nil {
			return nil, err
		}
		return listerat{file}, nil
	}

	return nil, errors.New("unsupported")
}

func (fs *root) readdir(pathname string) ([]os.FileInfo, error) {
	dir, err := fs.fetch(pathname)
	if err != nil {
		return nil, err
	}

	if !dir.IsDir() {
		return nil, syscall.ENOTDIR
	}

	var files []os.FileInfo

	for name, file := range fs.files {
		if path.Dir(name) == dir.name {
			files = append(files, file)
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	return files, nil
}

func (fs *root) Readlink(pathname string) (string, error) {
	file, err := fs.lfetch(pathname)
	if err != nil {
		return "", err
	}

	if file.symlink == "" {
		return "", os.ErrInvalid
	}

	return file.symlink, nil
}

// implements LstatFileLister interface
func (fs *root) Lstat(r *Request) (ListerAt, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	file, err := fs.lfetch(r.Filepath)
	if err != nil {
		return nil, err
	}
	return listerat{file}, nil
}

// In memory file-system-y thing that the Hanlders live on
type root struct {
	rootFile *memFile
	mockErr  error

	mu    sync.Mutex
	files map[string]*memFile
}

// Set a mocked error that the next handler call will return.
// Set to nil to reset for no error.
func (fs *root) returnErr(err error) {
	fs.mockErr = err
}

func (fs *root) lfetch(path string) (*memFile, error) {
	if path == "/" {
		return fs.rootFile, nil
	}

	file, ok := fs.files[path]
	if file == nil {
		if ok {
			delete(fs.files, path)
		}

		return nil, os.ErrNotExist
	}

	return file, nil
}

// canonName returns the “canonical” name of a file, that is:
// if the directory of the pathname is a symlink, it follows that symlink to the valid directory name.
// this is relatively easy, since `dir.name` will be the only valid canonical path for a directory.
func (fs *root) canonName(pathname string) (string, error) {
	dirname, filename := path.Dir(pathname), path.Base(pathname)

	dir, err := fs.fetch(dirname)
	if err != nil {
		return "", err
	}

	if !dir.IsDir() {
		return "", syscall.ENOTDIR
	}

	return path.Join(dir.name, filename), nil
}

func (fs *root) exists(path string) bool {
	path, err := fs.canonName(path)
	if err != nil {
		return false
	}

	_, err = fs.lfetch(path)

	return !errors.Is(err, os.ErrNotExist)
}

func (fs *root) fetch(pathname string) (*memFile, error) {
	file, err := fs.lfetch(pathname)
	if err != nil {
		return nil, err
	}

	var count int
	for file.symlink != "" {
		if count++; count > maxSymlinkFollows {
			return nil, errTooManySymlinks
		}

		linkTarget := file.symlink
		if !path.IsAbs(linkTarget) {
			linkTarget = path.Join(path.Dir(file.name), linkTarget)
		}

		file, err = fs.lfetch(linkTarget)
		if err != nil {
			return nil, err
		}
	}

	return file, nil
}

// Implements os.FileInfo, io.ReaderAt and io.WriterAt interfaces.
// These are the 3 interfaces necessary for the Handlers.
// Implements the optional interface TransferError.
type memFile struct {
	name    string
	modtime time.Time
	symlink string
	isdir   bool
	mode    uint32

	mu      sync.RWMutex
	content []byte
	err     error
}

// These are helper functions, they must be called while holding the memFile.mu mutex
func (f *memFile) size() int64  { return int64(len(f.content)) }
func (f *memFile) grow(n int64) { f.content = append(f.content, make([]byte, n)...) }

// Have memFile fulfill os.FileInfo interface
func (f *memFile) Name() string { return path.Base(f.name) }
func (f *memFile) Size() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.size()
}
func (f *memFile) Mode() os.FileMode {
	// Hardcoded values, because we do not even try to support changing the
	// mode of directories or symlinks.
	if f.isdir {
		return os.FileMode(0755) | os.ModeDir
	}
	if f.symlink != "" {
		return os.FileMode(0777) | os.ModeSymlink
	}
	return os.FileMode(f.mode)
}
func (f *memFile) ModTime() time.Time { return f.modtime }
func (f *memFile) IsDir() bool        { return f.isdir }
func (f *memFile) Sys() interface{} {
	return fakeFileInfoSys()
}

func (f *memFile) ReadAt(b []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return 0, f.err
	}

	if off < 0 {
		return 0, errors.New("memFile.ReadAt: negative offset")
	}

	if off >= f.size() {
		return 0, io.EOF
	}

	n := copy(b, f.content[off:])
	if n < len(b) {
		return n, io.EOF
	}

	return n, nil
}

func (f *memFile) WriteAt(b []byte, off int64) (int, error) {
	// fmt.Println(string(p), off)
	// mimic write delays, should be optional
	time.Sleep(time.Microsecond * time.Duration(len(b)))

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return 0, f.err
	}

	grow := int64(len(b)) + off - f.size()
	if grow > 0 {
		f.grow(grow)
	}

	return copy(f.content[off:], b), nil
}

func (f *memFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return f.err
	}

	grow := size - f.size()
	if grow <= 0 {
		f.content = f.content[:size]
	} else {
		f.grow(grow)
	}

	return nil
}

func (f *memFile) TransferError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.err = err
}
