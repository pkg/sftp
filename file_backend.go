package sftp

import (
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
	"io"
)

type LocalFile interface {
	io.ReaderAt
	io.WriterAt
	io.Closer

	Name() string
	Readdir(n int) (fi []os.FileInfo, err error)
	Truncate(size int64) error
	Chmod(mode os.FileMode) error
	Chown(uid, gid int) error
	Stat() (os.FileInfo, error)
}

type StorageBackend interface {
	NextHandle(LocalFile) string
	CloseHandle(string) error
	GetHandle(string) (LocalFile, bool)
	CloseAll()

	OpenFile(name string, flag int, perm os.FileMode) (LocalFile, error)
	Lstat(name string) (os.FileInfo, error)
	Stat(name string) (os.FileInfo, error)
	Mkdir(name string, perm os.FileMode) error
	Remove(name string) error
	Rename(oldpath, newpath string) error
	Symlink(oldname, newname string) error
	Readlink(name string) (string, error)
	Truncate(name string, size int64) error
	Chmod(name string, mode os.FileMode) error
	Chtimes(name string, atime time.Time, mtime time.Time) error
	Chown(name string, uid, gid int) error
}

type FileBackend struct {
	openFiles     map[string]LocalFile
	openFilesLock *sync.RWMutex
	handleCount   int
}

func NewFileBackend() StorageBackend {
	return &FileBackend{
		openFiles:     make(map[string]LocalFile),
		openFilesLock: &sync.RWMutex{},
	}

}

func (s *FileBackend) NextHandle(f LocalFile) string {
	s.openFilesLock.Lock()
	defer s.openFilesLock.Unlock()
	s.handleCount++
	handle := strconv.Itoa(s.handleCount)
	s.openFiles[handle] = f
	return handle
}

func (s *FileBackend) CloseHandle(handle string) error {
	s.openFilesLock.Lock()
	defer s.openFilesLock.Unlock()
	if f, ok := s.openFiles[handle]; ok {
		delete(s.openFiles, handle)
		return f.Close()
	}

	return syscall.EBADF
}

func (s *FileBackend) GetHandle(handle string) (LocalFile, bool) {
	s.openFilesLock.RLock()
	defer s.openFilesLock.RUnlock()
	f, ok := s.openFiles[handle]
	return f, ok
}

func (s *FileBackend) CloseAll() {
	for _, file := range s.openFiles {
		file.Close()
	}
}

func (s FileBackend) OpenFile(name string, flag int, perm os.FileMode) (LocalFile, error) {
	return os.OpenFile(name, flag, perm)
}

func (s FileBackend) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

func (s FileBackend) Mkdir(name string, perm os.FileMode) error {
	return os.Mkdir(name, perm)
}

func (s FileBackend) Remove(name string) error {
	return os.Remove(name)
}

func (s FileBackend) Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (s FileBackend) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

func (s FileBackend) Readlink(name string) (string, error) {
	return os.Readlink(name)
}

func (s FileBackend) Truncate(name string, size int64) error {
	return os.Truncate(name, size)
}

func (s FileBackend) Chmod(name string, mode os.FileMode) error {
	return os.Chmod(name, mode)
}

func (s FileBackend) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(name, atime, mtime)
}

func (s FileBackend) Chown(name string, uid, gid int) error {
	return os.Chown(name, uid, gid)
}

func (s FileBackend) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}