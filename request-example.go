package sftp

// This serves as an example of how to implement the request server handler as
// well as a dummy backend for testing. It implements an in-memory backend that
// works as a very simple filesystem with simple flat key-value lookup system.

import (
	"errors"
	"fmt"
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
		files: make(map[string]*memFile),
	}
	root.memFile = newMemFile("/", true)
	return Handlers{root, root, root, root}
}

// Example Handlers
func (fs *root) Fileread(r *Request) (io.ReaderAt, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	file, err := fs.openfile(r.Filepath, r.Flags)
	if err != nil {
		return nil, err
	}
	return file.ReaderAt()
}

func (fs *root) Filewrite(r *Request) (io.WriterAt, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	file, err := fs.openfile(r.Filepath, r.Flags)
	if err != nil {
		return nil, err
	}
	return file.WriterAt()
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
	if link, err := fs.lfetch(pathname); err == nil {
		// get the true name of the end-point of any symlinks.
		for err == nil {
			pathname = link.symlink
			link, err = fs.lfetch(pathname)
		}
	}

	dirname, filename := path.Dir(pathname), path.Base(pathname)

	dir, err := fs.fetchMaybeExclusive(dirname, exclusive)
	if err != nil {
		return nil, err
	}

	if !dir.isdir {
		return nil, syscall.ENOTDIR
	}

	pathname = path.Join(dir.name, filename)

	file := &memFile{
		name: pathname,
	}
	fs.files[pathname] = file

	return file, nil
}

func (fs *root) openfile(pathname string, flags uint32) (*memFile, error) {
	pflags := newFileOpenFlags(flags)

	file, err := fs.fetchMaybeExclusive(pathname, pflags.Creat && pflags.Excl)

	if err == os.ErrNotExist {
		if !pflags.Creat {
			return nil, os.ErrNotExist
		}

		return fs.newfile(pathname, pflags.Excl)
	}

	if err != nil {
		return nil, err
	}

	if pflags.Creat && pflags.Excl {
		return nil, os.ErrExist
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
		file, err := fs.fetch(r.Filepath)
		if err != nil {
			return err
		}
		if _, ok := fs.files[r.Target]; ok {
			return &os.LinkError{Op: "rename", Old: r.Filepath, New: r.Target,
				Err: fmt.Errorf("dest file exists")}
		}
		file.name = r.Target
		fs.files[r.Target] = file
		delete(fs.files, r.Filepath)

		if file.IsDir() {
			for path, file := range fs.files {
				if strings.HasPrefix(path, r.Filepath+"/") {
					file.name = r.Target + path[len(r.Filepath):]
					fs.files[r.Target+path[len(r.Filepath):]] = file
					delete(fs.files, path)
				}
			}
		}
	case "Rmdir", "Remove":
		file, err := fs.fetch(path.Dir(r.Filepath))
		if err != nil {
			return err
		}

		if file.IsDir() {
			for path := range fs.files {
				if strings.HasPrefix(path, r.Filepath+"/") {
					return &os.PathError{
						Op:   "remove",
						Path: r.Filepath + "/",
						Err:  fmt.Errorf("directory is not empty"),
					}
				}
			}
		}

		delete(fs.files, r.Filepath)

	case "Mkdir":
		dir, err := fs.newfile(r.Filepath, false)
		if err != nil {
			return err
		}

		dir.isdir = true

	case "Link":
		file, err := fs.fetch(r.Filepath)
		if err != nil {
			return err
		}

		if file.IsDir() {
			return fmt.Errorf("hard link not allowed for directory")
		}

		fs.files[r.Target] = file

	case "Symlink":
		_, err := fs.lfetch(r.Target)
		if err != os.ErrNotExist {
			return os.ErrExist
		}

		link, err := fs.newfile(r.Target, false)
		if err != nil {
			return err
		}

		link.symlink = r.Filepath
	}
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
	switch r.Method {
	case "List":
		return fs.ReadDir(r)
	case "Stat":
		return fs.Stat(r)
	case "Readlink":
		return fs.ReadLink(r)
	}
	return nil, nil
}

func (fs *root) readdir(pathname string) ([]os.FileInfo, error) {
	file, err := fs.fetch(pathname)
	if err != nil {
		return nil, err
	}

	if !file.IsDir() {
		return nil, syscall.ENOTDIR
	}

	var files []os.FileInfo

	for name, file := range fs.files {
		if path.Dir(name) == pathname {
			files = append(files, file)
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	return files, nil
}

func (fs *root) ReadDir(r *Request) (ListerAt, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	files, err := fs.readdir(r.Filepath)
	if err != nil {
		return nil, err
	}

	return listerat(files), nil
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

func (fs *root) ReadLink(r *Request) (ListerAt, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	symlink, err := fs.readlink(r.Filepath)

	file, err := fs.lfetch(symlink)
	if err != nil {
		// return a dummy memFile, with appropriate name.
		return listerat{&memFile{
			name: symlink,
			err:  os.ErrNotExist, // prevent accidental use as a reader/writer.
		}}, nil
	}

	return listerat{file}, nil
}

func (fs *root) Stat(r *Request) (ListerAt, error) {
	if fs.mockErr != nil {
		return nil, fs.mockErr
	}
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing

	fs.mu.Lock()
	defer fs.mu.Unlock()

	file, err := fs.fetch(r.Filepath)
	if err != nil {
		return nil, err
	}
	return listerat{file}, nil
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
	*memFile
	mockErr error

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
		return fs.memFile, nil
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

func (fs *root) fetch(path string) (*memFile, error) {
	return fs.fetchMaybeExclusive(path, false)
}

func (fs *root) fetchMaybeExclusive(path string, exclusive bool) (*memFile, error) {
	file, err := fs.lfetch(path)
	if err != nil {
		return nil, err
	}

	for file.symlink != "" {
		if exclusive {
			return nil, os.ErrInvalid
		}

		file, err = fs.lfetch(file.symlink)
		if err != nil {
			return nil, err
		}
	}

	return file, nil
}

// Implements os.FileInfo, Reader and Writer interfaces.
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

// factory to make sure modtime is set
func newMemFile(name string, isdir bool) *memFile {
	return &memFile{
		name:    name,
		modtime: time.Now(),
		isdir:   isdir,
	}
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

// Read/Write
func (f *memFile) ReaderAt() (io.ReaderAt, error) {
	if f.isdir {
		return nil, os.ErrInvalid
	}

	return f, nil
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

func (f *memFile) WriterAt() (io.WriterAt, error) {
	if f.isdir {
		return nil, os.ErrInvalid
	}

	return f, nil
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
