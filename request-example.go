package sftp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var _ = fmt.Println

type memFile struct {
	name    string
	content []byte
	modtime time.Time
	files   map[string]*memFile
	symlink string
	isdir   bool
}

func newMemFile(name string, isdir bool) *memFile {
	return &memFile{
		name:    name,
		modtime: time.Now(),
		files:   make(map[string]*memFile),
		isdir:   isdir,
	}
}

func InMemHandler() Handlers {
	root := newMemFile("/", true)
	return Handlers{root, root, root, root}
}

// Have memFile fulfill os.FileInfo interface
func (f *memFile) Name() string { return f.name }
func (f *memFile) Size() int64  { return int64(len(f.content)) }
func (f *memFile) Mode() os.FileMode {
	ret := os.ModePerm
	if f.isdir {
		ret = ret | os.ModeDir
	}
	return ret
}
func (f *memFile) ModTime() time.Time { return time.Now() }
func (f *memFile) IsDir() bool        { return f.isdir }
func (f *memFile) Sys() interface{}   { return nil }

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

// Filesystemy things
func (f *memFile) fetch(path string) (*memFile, error) {
	var file *memFile = f
	for _, name := range strings.Split(path, "/") {
		fmt.Print(name, " - ")
		var ok bool
		switch name {
		case "/":
		default:
			fmt.Println(file.files)
			if file, ok = file.files[name]; !ok {
				return nil, os.ErrNotExist
			}
		}
	}
	return file, nil
}

// Handlers
func (f *memFile) Fileread(r *Request) (io.Reader, error) {
	file, err := f.fetch(r.Filepath)
	if err != nil {
		return nil, err
	}
	if file.symlink != "" {
		file, err = f.fetch(file.symlink)
		if err != nil {
			return nil, err
		}
	}
	return file.Reader()
}

func (f *memFile) Filewrite(r *Request) (io.Writer, error) {
	file, err := f.fetch(r.Filepath)
	if err == os.ErrNotExist {
		dir, err := f.fetch(filepath.Dir(r.Filepath))
		if !dir.isdir {
			return nil, os.ErrInvalid
		}
		if err != nil {
			return nil, err
		}
		name := filepath.Base(r.Filepath)
		file = newMemFile(name, false)
		dir.files[name] = file
	}
	return file.Writer()
}

func (f *memFile) Filecmd(r *Request) error {
	switch r.Method {
	case "SetStat":
		return nil
	case "Rename":
		filename := filepath.Base(r.Filepath)
		orig_dir, err := f.fetch(filepath.Dir(r.Filepath))
		if err != nil {
			return err
		}
		dest, err := f.fetch(r.Target)
		if err != nil {
			return err
		}
		if !dest.isdir {
			return os.ErrInvalid
		}
		dest.files[filename] = orig_dir.files[filename]
		delete(orig_dir.files, filename)
	case "Rmdir", "Remove":
		parent, err := f.fetch(filepath.Dir(r.Filepath))
		if err != nil {
			return err
		}
		delete(parent.files, filepath.Base(r.Filepath))
	case "Mkdir":
		parent, err := f.fetch(filepath.Dir(r.Filepath))
		if err != nil {
			return err
		}
		name := filepath.Base(r.Filepath)
		file := newMemFile(name, true)
		parent.files[name] = file
	case "Symlink":
		fmt.Println("ln -s", r.Filepath, r.Target)
		parent, err := f.fetch(filepath.Dir(r.Filepath))
		if err != nil {
			return err
		}
		name := filepath.Base(r.Target)
		file := newMemFile(name, false)
		file.symlink = r.Filepath
		parent.files[name] = file
	}
	return nil
}

func (f *memFile) Fileinfo(r *Request) ([]os.FileInfo, error) {
	switch r.Method {
	case "List":
		file, err := f.fetch(r.Filepath)
		if err != nil {
			return nil, err
		}
		if file.isdir {
			list := make([]os.FileInfo, 0, len(file.files))
			for _, val := range file.files {
				fmt.Println(val)
				list = append(list, val)
			}
			return list, nil
		} else {
			return []os.FileInfo{file}, nil
		}
	case "Stat":
		file, err := f.fetch(r.Filepath)
		if err != nil {
			return nil, err
		}
		return []os.FileInfo{file}, nil
	case "Readlink":
		file, err := f.fetch(r.Filepath)
		if err != nil {
			return nil, err
		}
		if len(file.symlink) > 0 {
			file, err = f.fetch(file.symlink)
			if err != nil {
				return nil, err
			}
		}
		return []os.FileInfo{file}, nil
	}
	return nil, nil
}
