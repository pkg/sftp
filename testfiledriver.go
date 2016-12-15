package sftp

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

type TestFileDriver struct{}

func (d TestFileDriver) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (d TestFileDriver) ListDir(path string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(path)
}

func (d TestFileDriver) DeleteDir(path string) error {
	return os.Remove(path)
}

func (d TestFileDriver) DeleteFile(path string) error {
	return os.Remove(path)
}

func (d TestFileDriver) Rename(oldpath string, newpath string) error {
	return os.Rename(oldpath, newpath)
}

func (d TestFileDriver) MakeDir(path string) error {
	return os.Mkdir(path, 0755)
}

func (d TestFileDriver) GetFile(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	return f, err
}

func (d TestFileDriver) PutFile(path string, r io.Reader) error {
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, bytes, 0755)
}

func (d TestFileDriver) TranslatePath(root, homedir, path string) (string, error) {
	return filepath.Clean("/" + path)
}
