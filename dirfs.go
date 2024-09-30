package sftp

import (
	"io/fs"
	"path"
)

type fsys struct {
	dir string
	cl  *Client
}

// DirFS returns a file system (an fs.FS) for the tree of files rooted at the directory dir.
//
// Note that cl.DirFS("/prefix") only guarantees that the Open calls it makes to the remote system will begin with "/prefix":
// cl.DirFS("/prefix").Open("file") is the same as cl.Open("/prefix/file").
// So, if /prefix/file is a symbolic link pointing outside the /prefix tree,
// then using DirFS does not stop the access any more than using cl.Open does.
// DirFS is therefore not a general substitute for a chroot-style security mechanism when the directory tree contains arbitrary content.
//
// The directory dir must not be "".
//
// The result implements io/fs.StatFS, io/fs.ReadFileFS, and io/fs.ReadDirFS.
func (cl *Client) DirFS(dir string) fs.FS {
	return &fsys{
		dir: path.Clean(dir),
		cl:  cl,
	}
}

func (cl *fsys) join(name string) string {
	return path.Join(cl.dir, name)
}

// Open implements fs.FS.
func (cl *fsys) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	return cl.cl.Open(cl.join(name))
}

// ReadDir implements fs.ReadDirFS.
func (cl *fsys) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	return cl.cl.ReadDir(cl.join(name))
}

// ReadFile implements fs.ReadFileFS.
func (cl *fsys) ReadFile(name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}

	return cl.cl.ReadFile(cl.join(name))
}

// Stat implements fs.StatFS.
func (cl *fsys) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}

	return cl.cl.Stat(cl.join(name))
}

// Sub implements fs.SubFS.
func (cl *fsys) Sub(dir string) (fs.FS, error) {
	if !fs.ValidPath(dir) {
		return nil, &fs.PathError{Op: "sub", Path: dir, Err: fs.ErrInvalid}
	}

	if dir == "." {
		return cl, nil
	}

	return &fsys{
		dir: cl.join(dir),
		cl:  cl.cl,
	}, nil
}
