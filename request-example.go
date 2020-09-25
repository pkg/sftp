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

	return fs.OpenFile(r)
}

func (fs *root) Filewrite(r *Request) (io.WriterAt, error) {
	flags := r.Pflags()
	if !flags.Write {
		// sanity check
		return nil, os.ErrInvalid
	}

	return fs.OpenFile(r)
}

func (fs *root) OpenFile(r *Request) (WriterAtReaderAt, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	return fs.openfile(r.Filepath, r.Flags)
}

func (fs *root) newfile(pathname string, exclusive bool) (*memFile, error) {
	pathname, symlinked, err := fs.canonName(pathname)
	if err != nil {
		return nil, err
	}

	if exclusive && symlinked {
		return nil, os.ErrInvalid
	}

	switch pathname {
	case "", "/":
		// sanity check protection.
		return nil, os.ErrInvalid
	}

	if fs.files[pathname] != nil {
		return nil, os.ErrExist
	}

	file := &memFile{
		name:    pathname,
		modtime: time.Now(),
	}
	fs.files[pathname] = file

	return file, nil
}

func (fs *root) openfile(pathname string, flags uint32) (*memFile, error) {
	pflags := newFileOpenFlags(flags)

	file, err := fs.fetch(pathname)
	if err == os.ErrNotExist {
		if !pflags.Creat {
			return nil, os.ErrNotExist
		}

		// You can create files through symlinks.
		link, err := fs.lfetch(pathname)
		for err == nil && link.symlink != "" {
			if pflags.Excl {
				return nil, os.ErrInvalid
			}

			pathname = link.symlink
			link, err = fs.lfetch(pathname)
		}

		return fs.newfile(pathname, pflags.Excl)
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
		file, err := fs.fetch(r.Filepath)
		if err != nil {
			return err
		}

		if r.AttrFlags().Size {
			return file.Truncate(int64(r.Attributes().Size))
		}

		return nil

	case "Rename":
		// SSH SPEC: "It is an error if there already exists a file with the name specified by newpath."
		// This varies from the POSIX specification, that says it should replace the new file.
		if fs.exists(r.Target) {
			return &os.LinkError{
				Op:  "rename",
				Old: r.Filepath,
				New: r.Target,
				Err: os.ErrExist,
			}
		}

		return fs.rename(r.Filepath, r.Target)

	case "Rmdir":
		return fs.rmdir(r.Filepath)

	case "Remove":
		return fs.remove(r.Filepath)

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

	newpath, _, err = fs.canonName(newpath)
	if err != nil {
		return err
	}

	fs.files[newpath] = file

	if dir := file; dir.IsDir() {
		dirprefix := dir.name + "/"

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

func (fs *root) mkdir(pathname string) error {
	dir, err := fs.newfile(pathname, false)
	if err != nil {
		return err
	}

	dir.isdir = true

	return nil
}

func (fs *root) rmdir(pathname string) error {
	// does not follow symlinks!
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
			return &os.PathError{
				Op:   "rmdir",
				Path: pathname + "/",
				Err:  errors.New("directory is not empty"),
			}
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

	fs.files[newpath] = file

	return nil
}

// symlink() creates a symbolic link named `linkpath` which contains the string `target`.
// NOTE! This would be called with `symlink(req.Filepath, req.Target)` due to different semantics.
func (fs *root) symlink(target, linkpath string) error {
	_, err := fs.lfetch(linkpath)
	if err != os.ErrNotExist {
		return os.ErrExist
	}

	link, err := fs.newfile(linkpath, false)
	if err != nil {
		return err
	}

	link.symlink = target

	return nil
}

func (fs *root) remove(pathname string) error {
	// does not follow symlinks!
	file, err := fs.lfetch(pathname)
	if err != nil {
		return err
	}

	if file.IsDir() {
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

	case "Readlink":
		symlink, err := fs.readlink(r.Filepath)
		if err != nil {
			return nil, err
		}

		// SSH SPEC: The server will respond with a SSH_FXP_NAME packet containing only
		// one name and a dummy attributes value.
		return listerat{
			&memFile{
				name: symlink,
				err:  os.ErrNotExist, // prevent accidental use as a reader/writer.
			},
		}, nil
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

func (fs *root) readlink(pathname string) (string, error) {
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
func (fs *root) canonName(pathname string) (string, bool, error) {
	dirname, filename := path.Dir(pathname), path.Base(pathname)

	dir, err := fs.lfetch(dirname)
	if err != nil {
		return "", false, err
	}

	var symlinked bool

	for dir.symlink != "" {
		symlinked = true

		dir, err = fs.lfetch(dir.symlink)
		if err != nil {
			return "", symlinked, err
		}
	}

	if !dir.IsDir() {
		return "", symlinked, syscall.ENOTDIR
	}

	return path.Join(dir.name, filename), symlinked, nil
}

func (fs *root) exists(path string) bool {
	path, _, err := fs.canonName(path)
	if err != nil {
		return false
	}

	_, err = fs.lfetch(path)

	return err != os.ErrNotExist
}

func (fs *root) fetch(path string) (*memFile, error) {
	file, err := fs.lfetch(path)
	if err != nil {
		return nil, err
	}

	for file.symlink != "" {
		file, err = fs.lfetch(file.symlink)
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
	if f.isdir {
		return os.FileMode(0755) | os.ModeDir
	}
	if f.symlink != "" {
		return os.FileMode(0777) | os.ModeSymlink
	}
	return os.FileMode(0644)
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
