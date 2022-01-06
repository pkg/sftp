// +build !plan9

package sftp

// This serves as an example of how to implement the request server handler as
// well as a dummy backend for testing. It implements an in-memory backend that
// works as a very simple filesystem with simple flat key-value lookup system.

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ChrootHandler returns a Hanlders object with the specific root path.
func ChrootHandler(rootPath string) Handlers {
	chroot := &chroot{
		rootPath: rootPath,
	}
	return Handlers{chroot, chroot, chroot, chroot}
}

type chroot struct {
	rootPath string
}

func (fs *chroot) toOsFlags(flags uint32) (int, error) {
	var osFlags int
	if flags&sshFxfRead != 0 || flags&sshFxfWrite != 0 {
		osFlags |= os.O_RDWR
	} else if flags&sshFxfWrite != 0 {
		osFlags |= os.O_WRONLY
	} else if flags&sshFxfRead != 0 {
		osFlags |= os.O_RDONLY
	} else {
		// how are they opening?
		return 0, syscall.EINVAL
	}
	if flags&sshFxfCreat != 0 {
		osFlags |= os.O_CREATE
	}
	if flags&sshFxfTrunc != 0 {
		osFlags |= os.O_TRUNC
	}
	if flags&sshFxfExcl != 0 {
		osFlags |= os.O_EXCL
	}
	return osFlags, nil
}

const OUT_PATH_PREFIX = ".." + string(filepath.Separator)

func (fs *chroot) getRealPath(p string) (string, error) {
	p = filepath.Join(fs.rootPath, p)
	if fs.rootPath != "" && fs.rootPath != "/" {
		rel, err := filepath.Rel(fs.rootPath, p)
		if err != nil || strings.HasPrefix(rel, OUT_PATH_PREFIX) {
			return "", os.ErrInvalid
		}
	}
	return p, nil
}

func (fs *chroot) getRelativePath(p string) (string, error) {
	if fs.rootPath != "" && fs.rootPath != "/" {
		rel, err := filepath.Rel(fs.rootPath, p)
		if err != nil || strings.HasPrefix(rel, OUT_PATH_PREFIX) {
			return "", os.ErrInvalid
		}
		p = rel
	}
	return cleanPath(p), nil
}

func (fs *chroot) Fileread(r *Request) (io.ReaderAt, error) {
	if r.Flags&sshFxfRead == 0 {
		return nil, os.ErrInvalid
	}
	return fs.OpenFile(r)
}

func (fs *chroot) Filewrite(r *Request) (io.WriterAt, error) {
	if r.Flags&sshFxfWrite == 0 {
		return nil, os.ErrInvalid
	}
	return fs.OpenFile(r)
}

func (fs *chroot) OpenFile(r *Request) (WriterAtReaderAt, error) {
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing
	return fs.openfile(r.Filepath, r.Flags)
}

func (fs *chroot) openfile(pathname string, flags uint32) (*os.File, error) {
	osFlags, err := fs.toOsFlags(flags)
	if err != nil {
		return nil, err
	}
	realPath, err := fs.getRealPath(pathname)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(realPath, osFlags, 0644)
	if err != nil {
		return nil, err
	}
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if stat.IsDir() {
		return nil, os.ErrInvalid
	}
	return file, nil
}

func (fs *chroot) Filecmd(r *Request) error {
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing
	switch r.Method {
	case "Setstat":
		file, err := fs.openfile(r.Filepath, sshFxfWrite)
		if err != nil {
			return err
		}

		if r.AttrFlags().Size {
			return file.Truncate(int64(r.Attributes().Size))
		}

		return nil

	case "Rename":
		return fs.rename(r.Filepath, r.Target, false)

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

func (fs *chroot) rename(oldpath, newpath string, posix bool) error {
	oldpath, err := fs.getRealPath(oldpath)
	if err != nil {
		return err
	}
	newpath, err = fs.getRealPath(newpath)
	if err != nil {
		return err
	}
	// SFTP-v2: "It is an error if there already exists a file with the name specified by newpath."
	// This varies from the POSIX specification, which allows limited replacement of target files.
	if !posix {
		_, err := os.Lstat(newpath)
		if !os.IsNotExist(err) {
			return os.ErrExist
		}
	}
	return os.Rename(oldpath, newpath)
}

func (fs *chroot) PosixRename(r *Request) error {
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing
	return fs.rename(r.Filepath, r.Target, true)
}

func (fs *chroot) StatVFS(r *Request) (*StatVFS, error) {
	filepath, err := fs.getRealPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	return getStatVFSForPath(filepath)
}

func (fs *chroot) mkdir(pathname string) error {
	pathname, err := fs.getRealPath(pathname)
	if err != nil {
		return err
	}
	return os.Mkdir(pathname, 0755)
}

func (fs *chroot) rmdir(pathname string) error {
	pathname, err := fs.getRealPath(pathname)
	if err != nil {
		return err
	}
	stat, err := os.Lstat(pathname)
	if err != nil {
		return err
	}
	// IEEE 1003.1: If pathname is a symlink, then rmdir should fail with ENOTDIR.
	if stat.Mode()&os.ModeSymlink != 0 || !stat.IsDir() {
		return syscall.ENOTDIR
	}
	return os.Remove(pathname)
}

func (fs *chroot) link(oldpath, newpath string) error {
	// make sure user cannot escape root path
	oldpath, err := fs.getRealPath(oldpath)
	if err != nil {
		return err
	}
	newpath, err = fs.getRealPath(newpath)
	if err != nil {
		return err
	}
	return os.Link(oldpath, newpath)
}

// symlink() creates a symbolic link named `linkpath` which contains the string `target`.
// NOTE! This would be called with `symlink(req.Filepath, req.Target)` due to different semantics.
func (fs *chroot) symlink(target, linkpath string) error {
	// make sure user cannot escape root path
	target, err := fs.getRealPath(target)
	if err != nil {
		return err
	}
	linkpath, err = fs.getRealPath(linkpath)
	if err != nil {
		return err
	}
	return os.Symlink(target, linkpath)
}

func (fs *chroot) unlink(pathname string) error {
	pathname, err := fs.getRealPath(pathname)
	if err != nil {
		return err
	}
	stat, err := os.Lstat(pathname)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		// IEEE 1003.1: implementations may opt out of allowing the unlinking of directories.
		// SFTP-v2: SSH_FXP_REMOVE may not remove directories.
		return os.ErrInvalid
	}
	return os.Remove(pathname)
}

func (fs *chroot) Filelist(r *Request) (ListerAt, error) {
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing
	realPath, err := fs.getRealPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	switch r.Method {
	case "List":
		files, err := os.ReadDir(realPath)
		if err != nil {
			return nil, err
		}
		var res listerat = make(listerat, 0, len(files))
		for _, file := range files {
			stat, err := file.Info()
			if err != nil {
				return nil, err
			}
			res = append(res, stat)
		}
		return res, nil

	case "Stat":
		file, err := os.Stat(realPath)
		if err != nil {
			return nil, err
		}
		return listerat{file}, nil

	case "Readlink":
		symlink, err := os.Readlink(realPath)
		if err != nil {
			return nil, err
		}
		res, err := fs.getRelativePath(symlink)
		if err != nil {
			return nil, err
		}
		// SFTP-v2: The server will respond with a SSH_FXP_NAME packet containing only
		// one name and a dummy attributes value.
		return listerat{
			&chrootFile{
				memFile{
					name: res,
					err:  os.ErrNotExist, // prevent accidental use as a reader/writer.
				},
			},
		}, nil
	}

	return nil, errors.New("unsupported")
}

// implements LstatFileLister interface
func (fs *chroot) Lstat(r *Request) (ListerAt, error) {
	_ = r.WithContext(r.Context()) // initialize context for deadlock testing
	realPath, err := fs.getRealPath(r.Filepath)
	if err != nil {
		return nil, err
	}
	file, err := os.Lstat(realPath)
	if err != nil {
		return nil, err
	}
	return listerat{file}, nil
}

// implements RealpathFileLister interface
func (fs *chroot) Realpath(p string) string {
	return cleanPath(p)
}

type chrootFile struct {
	memFile
}

func (f *chrootFile) Name() string {
	// memFile.Name() only return file base name, not fit for ReadLink
	return f.name
}
