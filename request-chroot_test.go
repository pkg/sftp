// +build !plan9

package sftp

import (
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const outbound = "../outbound"

func getChrootTuple(t *testing.T) (string, ClientWrapper) {
	testRoot, err := ioutil.TempDir(os.TempDir(), "sftp")
	require.NoError(t, err)
	sftpRoot := filepath.Join(testRoot, "sftp")
	require.NoError(t, os.MkdirAll(sftpRoot, 0755))
	require.NoError(t, ioutil.WriteFile(filepath.Join(sftpRoot, outbound), []byte("outbound"), 0644))

	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	server := NewRequestServer(struct {
		io.Reader
		io.WriteCloser
	}{sr, sw}, ChrootHandler(sftpRoot))
	go func() {
		err := server.Serve()
		assert.ErrorIs(t, err, io.EOF)
	}()
	// NewClientPipe() will hung up until server.Serve(),
	// make sure start server first
	client, err := NewClientPipe(cr, cw)
	require.NoError(t, err)
	return sftpRoot, ClientWrapper{
		Client:   client,
		server:   server,
		testRoot: testRoot,
	}
}

func AssertDirExist(t *testing.T, absPath string) bool {
	f, err := os.Lstat(absPath)
	return assert.NoError(t, err) && assert.True(t, f.IsDir())
}

func AssertNoDirExist(t *testing.T, absPath string) bool {
	_, err := os.Lstat(absPath)
	return assert.True(t, os.IsNotExist(err))
}

type ClientWrapper struct {
	*Client
	server   *RequestServer
	testRoot string
}

func (w ClientWrapper) Close(t *testing.T) {
	assert.NoError(t, w.server.Close())
	assert.NoError(t, w.Client.Close())
	assert.NoError(t, os.RemoveAll(w.testRoot))
}

func TestChrootStat(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)
	const (
		regular = "regular"
		symlink = "symlink"
		outlink = "outlink"
	)
	// prepare, create file and symlink for stat
	content := []byte(strings.Repeat("hello sftp", 1024))
	require.NoError(t, ioutil.WriteFile(filepath.Join(rootPath, regular), content, 0644))
	require.NoError(t, os.Symlink(filepath.Join(rootPath, regular), filepath.Join(rootPath, symlink)))
	require.NoError(t, os.Symlink(filepath.Join(rootPath, outbound), filepath.Join(rootPath, outlink)))

	// stat a regular file
	f, err := client.Stat(regular)
	if assert.NoError(t, err) {
		assert.EqualValues(t, regular, f.Name())
		assert.EqualValues(t, len(content), f.Size())
		assert.True(t, f.Mode().IsRegular())
	}

	// stat a symlink file
	f, err = client.Stat(symlink)
	if assert.NoError(t, err) {
		assert.EqualValues(t, symlink, f.Name())
		// Stat() should retrieve file info which symlink point to
		assert.EqualValues(t, len(content), f.Size())
		assert.True(t, f.Mode().IsRegular())
	}

	// outer path will be converted relate to root, and /{root}/outbound is not exist
	_, err = client.Lstat(outbound)
	assert.ErrorIs(t, err, os.ErrNotExist)

	// but we allow server to use symlink point to an outer file
	f, err = client.Stat(outlink)
	if assert.NoError(t, err) {
		assert.EqualValues(t, outlink, f.Name())
		assert.True(t, f.Mode().IsRegular())
	}
}

func TestChrootLstat(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)
	const (
		regular = "regular"
		symlink = "symlink"
	)
	// prepare, create file and symlink for stat
	content := []byte(strings.Repeat("hello sftp", 1024))
	require.NoError(t, ioutil.WriteFile(filepath.Join(rootPath, regular), content, 0644))
	require.NoError(t, os.Symlink(filepath.Join(rootPath, regular), filepath.Join(rootPath, symlink)))

	// lstat a regular file
	f, err := client.Lstat(regular)
	if assert.NoError(t, err) {
		// same as Stat()
		assert.EqualValues(t, regular, f.Name())
		assert.EqualValues(t, len(content), f.Size())
		assert.True(t, f.Mode().IsRegular())
	}

	// lstat a symlink file
	f, err = client.Lstat(symlink)
	if assert.NoError(t, err) {
		assert.EqualValues(t, symlink, f.Name())
		// we don't care symlink's size
		// different to Stat(), Lstat() read symlink itself
		assert.Equal(t, os.ModeSymlink, f.Mode().Type())
	}

	// outer path will be converted relate to root, and /{root}/outbound is not exist
	_, err = client.Lstat(outbound)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestChrootReadLink(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)
	const (
		regular = "/path/regular"
		symlink = "symlink"
		outlink = "outlink"
	)
	// prepare, create file and symlink for stat
	content := []byte(strings.Repeat("hello sftp", 1024))
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(rootPath, regular)), 0755))
	require.NoError(t, ioutil.WriteFile(filepath.Join(rootPath, regular), content, 0644))
	require.NoError(t, os.Symlink(filepath.Join(rootPath, regular), filepath.Join(rootPath, symlink)))
	require.NoError(t, os.Symlink(filepath.Join(rootPath, outbound), filepath.Join(rootPath, outlink)))

	// ReadLink should return symlink's target path
	f, err := client.ReadLink(symlink)
	if assert.NoError(t, err) {
		assert.Equal(t, regular, f)
	}

	// we don't want client user escape the root
	_, err = client.ReadLink(outbound)
	assert.Error(t, err)

	// cannot ReadLink a symlink point to outer file
	_, err = client.ReadLink(outlink)
	assert.Error(t, err)
}

func TestChrootRealPath(t *testing.T) {
	_, client := getChrootTuple(t)
	defer client.Close(t)

	p, err := client.RealPath(".")
	if assert.NoError(t, err) {
		assert.Equal(t, "/", p)
	}
	// outer path will be converted relate to root, so "/../" equals "/"
	p, err = client.RealPath("..")
	if assert.NoError(t, err) {
		assert.Equal(t, "/", p)
	}
	p, err = client.RealPath("./some/path/../to/../here")
	if assert.NoError(t, err) {
		assert.Equal(t, "/some/here", p)
	}
}

func TestChrootMkdir(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)

	const (
		dirPath    = "/mkdir"
		nestedPath = "/nested/should/fail"
	)

	assert.NoError(t, client.Mkdir(dirPath))
	// dir should be created
	AssertDirExist(t, filepath.Join(rootPath, dirPath))
	// cannot create nested dir
	assert.Error(t, client.Mkdir(nestedPath))
	AssertNoDirExist(t, filepath.Join(rootPath, nestedPath))
	// nested dir can create by MkdirAll
	assert.NoError(t, client.MkdirAll(nestedPath))
	AssertDirExist(t, filepath.Join(rootPath, nestedPath))

	// outer path will be converted relate to root
	assert.NoError(t, client.Mkdir("../newdir"))
	AssertDirExist(t, filepath.Join(rootPath, "newdir"))
}

func TestChrootRmdir(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)

	const (
		dirPath    = "/rmdir"
		nestedFile = "/rmdir/nested"
	)
	// prepare
	require.NoError(t, os.MkdirAll(filepath.Join(rootPath, dirPath), 0755))
	require.NoError(t, ioutil.WriteFile(filepath.Join(rootPath, nestedFile), []byte("some file"), 0644))
	// cannot remove a file
	assert.Error(t, client.RemoveDirectory(nestedFile))
	// cannot remove a dir not empty
	assert.Error(t, client.RemoveDirectory(dirPath))
	// remove nested file, than remove an empty dir
	require.NoError(t, os.Remove(filepath.Join(rootPath, nestedFile)))
	assert.NoError(t, client.RemoveDirectory(dirPath))
	// dir should be removed
	AssertNoDirExist(t, filepath.Join(rootPath, dirPath))

	// outer path will be converted relate to root, and /{root}/outbound is not exist
	assert.ErrorIs(t, client.RemoveDirectory("../outbound"), os.ErrNotExist)
}

func TestChrootRead(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)

	const (
		fileName = "writefile"
		dirName  = "dir"
	)
	content := []byte(strings.Repeat("hello sftp", 1024))
	require.NoError(t, ioutil.WriteFile(filepath.Join(rootPath, fileName), content, 0644))
	// we can read file from SFTP, and content should be same as local file
	file, err := client.Open(fileName)
	if assert.NoError(t, err) {
		defer file.Close()
		bytes, err := ioutil.ReadAll(file)
		if assert.NoError(t, err) {
			assert.EqualValues(t, content, bytes)
		}
	}
	// cannot open a dir
	require.NoError(t, os.MkdirAll(filepath.Join(rootPath, dirName), 0755))
	_, err = client.Open(dirName)
	assert.Error(t, err)

	// outer path will be converted relate to root, and /{root}/outbound is not exist
	_, err = client.Open(outbound)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestChrootWrite(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)

	const (
		fileName = "writefile"
		dirName  = "dir"
	)
	require.NoError(t, os.MkdirAll(filepath.Join(rootPath, dirName), 0755))
	writeOverSFTP := func(t *testing.T, file string, content []byte) bool {
		f, err := client.Create(file)
		if assert.NoError(t, err) {
			defer f.Close()
			n, err := f.Write(content)
			return assert.NoError(t, err) &&
				assert.EqualValues(t, len(content), n)
		}
		return false
	}
	checkLocalFile := func(t *testing.T, file string, content []byte) {
		bytes, err := ioutil.ReadFile(filepath.Join(rootPath, file))
		if assert.NoError(t, err) {
			assert.EqualValues(t, content, bytes)
		}
	}
	content := []byte(strings.Repeat("hello sftp", 1024))
	// cannot write dir
	_, err := client.Create(dirName)
	assert.Error(t, err)
	// write a new file and check result
	if writeOverSFTP(t, fileName, content) {
		checkLocalFile(t, fileName, content)
	}
	// truncate and write an exist file
	content = []byte(strings.Repeat("new content", 1024))
	if writeOverSFTP(t, fileName, content) {
		checkLocalFile(t, fileName, content)
	}
	// outer path will be converted relate to root
	f, err := client.Create(outbound)
	f.Close()
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(rootPath, filepath.Base(outbound)))
}

func TestChrootRemove(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)

	const (
		dirName  = "dir"
		fileName = "file-to-remove"
	)
	// prepare
	require.NoError(t, os.MkdirAll(filepath.Join(rootPath, dirName), 0755))
	require.NoError(t, ioutil.WriteFile(filepath.Join(rootPath, fileName), []byte{}, 0644))
	// file should be removed
	assert.NoError(t, client.Remove(fileName))
	assert.NoFileExists(t, filepath.Join(rootPath, fileName))
	// remove non-exist file should fail
	assert.Error(t, client.Remove(fileName))
	// can remove a folder
	assert.NoError(t, client.Remove(dirName))
	AssertNoDirExist(t, filepath.Join(rootPath, dirName))

	// outer path will be converted relate to root, and /{root}/outbound is not exist
	assert.ErrorIs(t, client.Remove(outbound), os.ErrNotExist)
}

func TestChrootRename(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)

	const (
		oldfile = "oldfile"
		newfile = "newfile"
		extfile = "extfile"
		olddir  = "olddir"
		newdir  = "newdir"
	)
	require.NoError(t, os.MkdirAll(filepath.Join(rootPath, olddir), 0755))
	require.NoError(t, ioutil.WriteFile(filepath.Join(rootPath, oldfile), []byte{}, 0644))
	require.NoError(t, ioutil.WriteFile(filepath.Join(rootPath, extfile), []byte{}, 0644))
	// call sftp cmd
	assert.NoError(t, client.Rename(oldfile, newfile))
	// check result, ori file should rename to new
	assert.NoFileExists(t, filepath.Join(rootPath, oldfile))
	assert.FileExists(t, filepath.Join(rootPath, newfile))
	// unlike POSIX, SFTP forbid rename to an exist file
	assert.Error(t, client.Rename(newfile, extfile))
	// can rename a folder
	assert.NoError(t, client.Rename(olddir, newdir))
	AssertDirExist(t, filepath.Join(rootPath, newdir))

	// we don't want client user escape the root
	assert.Error(t, client.Rename(outbound, olddir))
}

func TestChrootSymlink(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)

	const (
		regular = "/path/regular"
		symlink = "symlink"
	)
	// prepare
	content := []byte(strings.Repeat("hello sftp", 1024))
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(rootPath, regular)), 0755))
	require.NoError(t, ioutil.WriteFile(filepath.Join(rootPath, regular), content, 0644))
	// call sftp cmd
	assert.NoError(t, client.Symlink(regular, symlink))
	// symlink should be created
	f, err := os.Lstat(filepath.Join(rootPath, symlink))
	if assert.NoError(t, err) {
		assert.EqualValues(t, filepath.Base(symlink), f.Name())
		assert.Equal(t, fs.ModeSymlink, f.Mode().Type())
	}
	// cannot symlink out of root
	assert.Error(t, client.Symlink(outbound, symlink))
}

func TestChrootReadDir(t *testing.T) {
	rootPath, client := getChrootTuple(t)
	defer client.Close(t)
	const (
		FILE_CNT = 10
		MAX_SIZE = 1024
	)
	rand.Seed(time.Now().UnixNano())
	files := make(map[string]int)
	bytes := make([]byte, MAX_SIZE)
	require.NoError(t, os.MkdirAll(filepath.Join(rootPath, "readdir"), 0755))
	// TODO: generate complicate file tree
	for i := 0; i < FILE_CNT; i++ {
		fileName := fmt.Sprintf("file_%d", i)
		filePath := filepath.Join(rootPath, "readdir", fileName)
		fileSize := rand.Intn(MAX_SIZE)
		require.NoError(t, ioutil.WriteFile(filePath, bytes[:fileSize], 0644))
		files[fileName] = fileSize
	}

	stats, err := client.ReadDir("/readdir")
	require.NoError(t, err)
	res := make(map[string]int)
	for _, stat := range stats {
		res[stat.Name()] = int(stat.Size())
	}
	assert.EqualValues(t, files, res)
}
