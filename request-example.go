package sftp

// This serves as an example of how to implement the request server handler as
// well as a dummy backend for testing. It implements an in-memory backend that
// works as a very simple filesystem with simple flat key-value lookup system.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

var _ = fmt.Println

// Returns a Hanlders object with the test handlers
func InMemHandler() Handlers {
	root := &root{
		files: make(map[string]*memFile),
	}
	root.memFile = newMemFile("/", true)
	return Handlers{root, root, root, root}
}

// Handlers
func (fs *root) Fileread(r *Request) (io.Reader, error) {
	file, err := fs.fetch(r.Filepath)
	if err != nil {
		return nil, err
	}
	if file.symlink != "" {
		file, err = fs.fetch(file.symlink)
		if err != nil {
			return nil, err
		}
	}
	return file.Reader()
}

func (fs *root) Filewrite(r *Request) (io.Writer, error) {
	file, err := fs.fetch(r.Filepath)
	if err == os.ErrNotExist {
		dir, err := fs.fetch(filepath.Dir(r.Filepath))
		if err != nil {
			return nil, err
		}
		if !dir.isdir {
			return nil, os.ErrInvalid
		}
		file = newMemFile(r.Filepath, false)
		fs.files[r.Filepath] = file
	}
	return file.Writer()
}

func (fs *root) Filecmd(r *Request) error {
	switch r.Method {
	case "SetStat":
		return nil
	case "Rename":
		file, err := fs.fetch(r.Filepath)
		if err != nil {
			return err
		}
		if _, ok := fs.files[r.Target]; ok {
			return &os.LinkError{"rename", r.Filepath, r.Target,
				fmt.Errorf("dest file exists")}
		}
		fs.files[r.Target] = file
		delete(fs.files, r.Filepath)
	case "Rmdir", "Remove":
		_, err := fs.fetch(filepath.Dir(r.Filepath))
		if err != nil {
			return err
		}
		delete(fs.files, r.Filepath)
	case "Mkdir":
		_, err := fs.fetch(filepath.Dir(r.Filepath))
		if err != nil {
			return err
		}
		fs.files[r.Filepath] = newMemFile(r.Filepath, true)
	case "Symlink":
		fmt.Println("ln -s", r.Filepath, r.Target)
		_, err := fs.fetch(r.Filepath)
		if err != nil {
			return err
		}
		link := newMemFile(r.Target, false)
		link.symlink = r.Filepath
		fs.files[r.Target] = link
	}
	return nil
}

func (fs *root) Fileinfo(r *Request) ([]os.FileInfo, error) {
	switch r.Method {
	case "List":
		list := []os.FileInfo{}
		for fn, fi := range fs.files {
			if filepath.Dir(fn) == r.Filepath {
				fmt.Println(fn, fi.Name())
				list = append(list, fi)
			}
		}
		return list, nil
	case "Stat":
		file, err := fs.fetch(r.Filepath)
		if err != nil {
			// mimic os.Stat() return error
			return nil, &os.PathError{"stat", r.Filepath, syscall.ENOENT}
		}
		return []os.FileInfo{file}, nil
	case "Readlink":
		file, err := fs.fetch(r.Filepath)
		if err != nil {
			return nil, err
		}
		if file.symlink != "" {
			file, err = fs.fetch(file.symlink)
			if err != nil {
				return nil, err
			}
		}
		return []os.FileInfo{file}, nil
	}
	return nil, nil
}

// In memory file-system-y thing that the Hanlders live on
type root struct {
	*memFile
	files map[string]*memFile
}

func (r *root) fetch(path string) (*memFile, error) {
	if path == "/" {
		return r.memFile, nil
	}
	if file, ok := r.files[path]; ok {
		return file, nil
	}
	return nil, os.ErrNotExist
}

// Implements os.FileInfo, Reader and Writer interfaces.
// These are the 3 interfaces necessary for the Handlers.
type memFile struct {
	name    string
	content []byte
	modtime time.Time
	symlink string
	isdir   bool
}

// factory to make sure modtime is set
func newMemFile(name string, isdir bool) *memFile {
	return &memFile{
		name:    name,
		modtime: time.Now(),
		isdir:   isdir,
	}
}

// Have memFile fulfill os.FileInfo interface
func (f *memFile) Name() string { return filepath.Base(f.name) }
func (f *memFile) Size() int64  { return int64(len(f.content)) }
func (f *memFile) Mode() os.FileMode {
	ret := os.FileMode(0644)
	if f.isdir {
		ret = os.FileMode(0755) | os.ModeDir
	}
	if f.symlink != "" {
		ret = os.FileMode(0777) | os.ModeSymlink
	}
	return ret
}
func (f *memFile) ModTime() time.Time { return time.Now() }
func (f *memFile) IsDir() bool        { return f.isdir }
func (f *memFile) Sys() interface{} {
	return &syscall.Stat_t{Uid: 65534, Gid: 65534}
}

// Read/Write
func (f *memFile) Reader() (io.Reader, error) {
	if f.isdir {
		return nil, os.ErrInvalid
	}
	return bytes.NewReader(f.content), nil
}

func (f *memFile) Writer() (io.Writer, error) {
	if f.isdir {
		return nil, os.ErrInvalid
	}
	return f, nil
}
func (f *memFile) Write(p []byte) (int, error) {
	f.content = append(f.content, p...)
	return len(p), nil
}
