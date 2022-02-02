package sftp

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FilesystemHandler returns a Handlers object with the test handlers.
func FilesystemHandler(root string) Handlers {
	r := &fsRoot{
		root: root,
	}
	return Handlers{r, r, r, r}
}

type fsRoot struct {
	root string
}

func (fs *fsRoot) path(path string) string {
	// Make sure we do not escape the root:
	rooted := filepath.Clean("/" + path)
	return filepath.Join(fs.root, rooted)
}

func (fs *fsRoot) Fileread(r *Request) (io.ReaderAt, error) {
	flags := r.Pflags()
	if !flags.Read {
		// sanity check
		return nil, os.ErrInvalid
	}

	return os.Open(fs.path(r.Filepath))
}

func (fs *fsRoot) Filewrite(r *Request) (io.WriterAt, error) {
	flags := r.Pflags()
	if !flags.Write {
		// sanity check
		return nil, os.ErrInvalid
	}

	return os.OpenFile(fs.path(r.Filepath), sftpFlagsToOsFlags(flags), 0644)
}

func (fs *fsRoot) Filecmd(r *Request) error {
	switch r.Method {
	case "Setstat":
		if r.AttrFlags().Size {
			return os.Truncate(fs.path(r.Filepath), int64(r.Attributes().Size))
		}

		if r.AttrFlags().Acmodtime {
			return os.Chtimes(fs.path(r.Filepath), time.Now(), time.Unix(int64(r.Attributes().Mtime), 0))
		}

		return nil

	case "Rename":
		// SFTP-v2: "It is an error if there already exists a file with the name specified by newpath."
		// This varies from the POSIX specification, which allows limited replacement of target files.
		if _, statErr := os.Stat(fs.path(r.Filepath)); statErr == nil {
			return os.ErrExist
		}

		return os.Rename(fs.path(r.Filepath), fs.path(r.Target))

	case "Rmdir":
		return os.RemoveAll(fs.path(r.Filepath))

	case "Remove":
		// IEEE 1003.1 remove explicitly can unlink files and remove empty directories.
		// We use instead here the semantics of unlink, which is allowed to be restricted against directories.
		return os.Remove(fs.path(r.Filepath))

	case "Mkdir":
		return os.Mkdir(fs.path(r.Filepath), 0755)

	case "Link":
		return os.Link(fs.path(r.Filepath), fs.path(r.Target))

	case "Symlink":
		return os.Symlink(fs.path(r.Filepath), fs.path(r.Target))
	}

	return errors.New("unsupported")
}

func (fs *fsRoot) PosixRename(r *Request) error {
	return os.Rename(fs.path(r.Filepath), fs.path(r.Target))
}

func (fs *fsRoot) StatVFS(r *Request) (*StatVFS, error) {
	return getStatVFSForPath(fs.path(r.Filepath))
}

func (fs *fsRoot) Filelist(r *Request) (ListerAt, error) {
	switch r.Method {
	case "List":
		entries, err := os.ReadDir(fs.path(r.Filepath))
		if err != nil {
			return nil, err
		}

		fis := make([]os.FileInfo, 0, len(entries))
		for i := range entries {
			fi, err := entries[i].Info()
			if err != nil {
				return listerat(fis), err
			}
			fis = append(fis, fi)
		}
		return listerat(fis), nil

	case "Stat":
		file, err := os.Stat(fs.path(r.Filepath))
		if err != nil {
			return nil, err
		}
		return listerat{file}, nil

	case "Readlink":
		symlink, err := os.Readlink(fs.path(r.Filepath))
		if err != nil {
			return nil, err
		}

		// SFTP-v2: The server will respond with a SSH_FXP_NAME packet containing only
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

func sftpFlagsToOsFlags(in FileOpenFlags) int {
	out := os.O_WRONLY
	if in.Creat {
		out |= os.O_CREATE
	}

	// Don't use O_APPEND flag as it conflicts with WriteAt.
	// The sshFxfAppend flag is a no-op here as the client sends the offsets.

	if in.Excl {
		out |= os.O_EXCL
	}
	if in.Trunc {
		out |= os.O_TRUNC
	}
	return out
}
