package sftp

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

type DefaultFSBackend struct {
	log           DebugLogger
	openFiles     map[string]*os.File
	openFilesLock sync.RWMutex
	handleCount   int
}

func (be *DefaultFSBackend) RespondToOpenPacket(s *Server, p *SSHFxpOpenPacket) error {
	var osFlags int
	if p.HasPflags(ssh_FXF_READ, ssh_FXF_WRITE) {
		osFlags |= os.O_RDWR
	} else if p.HasPflags(ssh_FXF_WRITE) {
		osFlags |= os.O_WRONLY
	} else if p.HasPflags(ssh_FXF_READ) {
		osFlags |= os.O_RDONLY
	} else {
		// how are they opening?
		return s.SendError(p, syscall.EINVAL)
	}

	if p.HasPflags(ssh_FXF_APPEND) {
		osFlags |= os.O_APPEND
	}
	if p.HasPflags(ssh_FXF_CREAT) {
		osFlags |= os.O_CREATE
	}
	if p.HasPflags(ssh_FXF_TRUNC) {
		osFlags |= os.O_TRUNC
	}
	if p.HasPflags(ssh_FXF_EXCL) {
		osFlags |= os.O_EXCL
	}

	f, err := os.OpenFile(p.Path, osFlags, 0644)
	if err != nil {
		return s.SendError(p, err)
	}

	handle := be.NextHandle(f)
	return s.SendPacket(SSHFxpHandlePacket{p.ID, handle})
}

func (be *DefaultFSBackend) RespondToReaddirPacket(s *Server, p *SSHFxpReaddirPacket) error {
	f, ok := be.GetHandle(p.Handle)
	if !ok {
		return s.SendError(p, syscall.EBADF)
	}

	dirname := f.Name()
	dirents, err := f.Readdir(128)
	if err != nil {
		return s.SendError(p, err)
	}

	ret := SSHFxpNamePacket{ID: p.ID}
	for _, dirent := range dirents {
		ret.NameAttrs = append(ret.NameAttrs, SSHFxpNameAttr{
			Name:     dirent.Name(),
			LongName: RunLs(dirname, dirent),
			Attrs:    []interface{}{dirent},
		})
	}
	return s.SendPacket(ret)
}

func (be *DefaultFSBackend) RespondToSetstatPacket(s *Server, p *SSHFxpSetstatPacket) error {
	// additional unmarshalling is required for each possibility here
	b := p.Attrs.([]byte)
	var err error

	be.log.Debugf("setstat name \"%s\"", p.Path)
	if (p.Flags & ssh_FILEXFER_ATTR_SIZE) != 0 {
		var size uint64
		if size, b, err = unmarshalUint64Safe(b); err == nil {
			err = os.Truncate(p.Path, int64(size))
		}
	}
	if (p.Flags & ssh_FILEXFER_ATTR_PERMISSIONS) != 0 {
		var mode uint32
		if mode, b, err = unmarshalUint32Safe(b); err == nil {
			err = os.Chmod(p.Path, os.FileMode(mode))
		}
	}
	if (p.Flags & ssh_FILEXFER_ATTR_ACMODTIME) != 0 {
		var atime uint32
		var mtime uint32
		if atime, b, err = unmarshalUint32Safe(b); err != nil {
		} else if mtime, b, err = unmarshalUint32Safe(b); err != nil {
		} else {
			atimeT := time.Unix(int64(atime), 0)
			mtimeT := time.Unix(int64(mtime), 0)
			err = os.Chtimes(p.Path, atimeT, mtimeT)
		}
	}
	if (p.Flags & ssh_FILEXFER_ATTR_UIDGID) != 0 {
		var uid uint32
		var gid uint32
		if uid, b, err = unmarshalUint32Safe(b); err != nil {
		} else if gid, _, err = unmarshalUint32Safe(b); err != nil {
		} else {
			err = os.Chown(p.Path, int(uid), int(gid))
		}
	}

	return s.SendError(p, err)
}

func (be *DefaultFSBackend) RespondToFsetstatPacket(s *Server, p *SSHFxpFsetstatPacket) error {
	f, ok := be.GetHandle(p.Handle)
	if !ok {
		return s.SendError(p, syscall.EBADF)
	}

	// additional unmarshalling is required for each possibility here
	b := p.Attrs.([]byte)
	var err error

	be.log.Debugf("fsetstat name \"%s\"", f.Name())
	if (p.Flags & ssh_FILEXFER_ATTR_SIZE) != 0 {
		var size uint64
		if size, b, err = unmarshalUint64Safe(b); err == nil {
			err = f.Truncate(int64(size))
		}
	}
	if (p.Flags & ssh_FILEXFER_ATTR_PERMISSIONS) != 0 {
		var mode uint32
		if mode, b, err = unmarshalUint32Safe(b); err == nil {
			err = f.Chmod(os.FileMode(mode))
		}
	}
	if (p.Flags & ssh_FILEXFER_ATTR_ACMODTIME) != 0 {
		var atime uint32
		var mtime uint32
		if atime, b, err = unmarshalUint32Safe(b); err != nil {
		} else if mtime, b, err = unmarshalUint32Safe(b); err != nil {
		} else {
			atimeT := time.Unix(int64(atime), 0)
			mtimeT := time.Unix(int64(mtime), 0)
			err = os.Chtimes(f.Name(), atimeT, mtimeT)
		}
	}
	if (p.Flags & ssh_FILEXFER_ATTR_UIDGID) != 0 {
		var uid uint32
		var gid uint32
		if uid, b, err = unmarshalUint32Safe(b); err != nil {
		} else if gid, _, err = unmarshalUint32Safe(b); err != nil {
		} else {
			err = f.Chown(int(uid), int(gid))
		}
	}

	return s.SendError(p, err)
}

func (be *DefaultFSBackend) RespondToExtendedPacket(s *Server, p *SSHFxpExtendedPacket) error {
	if p.SpecificPacket == nil {
		return nil
	}
	return be.Handle(s, p.SpecificPacket)
}

func (be *DefaultFSBackend) RespondToExtendedPacketPosixRenamePacket(s *Server, p *SSHFxpExtendedPacketPosixRename) error {
	err := os.Rename(p.Oldpath, p.Newpath)
	return s.SendError(p, err)
}

func (be *DefaultFSBackend) RespondToExtendedPacketStatVFS(s *Server, p *SSHFxpExtendedPacketStatVFS) error {
	stat := &syscall.Statfs_t{}
	if err := syscall.Statfs(p.Path, stat); err != nil {
		return s.SendError(p, err)
	}

	retPkt, err := statvfsFromStatfst(stat)
	if err != nil {
		return s.SendError(p, err)
	}
	retPkt.ID = p.ID

	return s.SendPacket(retPkt)
}

func (be *DefaultFSBackend) NextHandle(f *os.File) string {
	be.openFilesLock.Lock()
	defer be.openFilesLock.Unlock()
	be.handleCount++
	handle := strconv.Itoa(be.handleCount)
	be.openFiles[handle] = f
	return handle
}

func (be *DefaultFSBackend) CloseHandle(handle string) error {
	be.openFilesLock.Lock()
	defer be.openFilesLock.Unlock()
	if f, ok := be.openFiles[handle]; ok {
		delete(be.openFiles, handle)
		return f.Close()
	}

	return syscall.EBADF
}

func (be *DefaultFSBackend) GetHandle(handle string) (*os.File, bool) {
	be.openFilesLock.RLock()
	defer be.openFilesLock.RUnlock()
	f, ok := be.openFiles[handle]
	return f, ok
}

func NewDefaultFSBackend(log DebugLogger) *DefaultFSBackend {
	return &DefaultFSBackend{
		log:       log,
		openFiles: make(map[string]*os.File),
	}
}

func (be *DefaultFSBackend) Handle(s *Server, p RequestPacket) error {
	switch p := p.(type) {
	case *SSHFxInitPacket:
		return s.SendPacket(SSHFxVersionPacket{sftpProtocolVersion, nil})
	case *SSHFxpStatPacket:
		// stat the requested file
		info, err := os.Stat(p.Path)
		if err != nil {
			return s.SendError(p, err)
		}
		return s.SendPacket(SSHFxpStatResponse{
			ID:   p.ID,
			Info: info,
		})
	case *SSHFxpLstatPacket:
		// stat the requested file
		info, err := os.Lstat(p.Path)
		if err != nil {
			return s.SendError(p, err)
		}
		return s.SendPacket(SSHFxpStatResponse{
			ID:   p.ID,
			Info: info,
		})
	case *SSHFxpFstatPacket:
		f, ok := be.GetHandle(p.Handle)
		if !ok {
			return s.SendError(p, syscall.EBADF)
		}

		info, err := f.Stat()
		if err != nil {
			return s.SendError(p, err)
		}

		return s.SendPacket(SSHFxpStatResponse{
			ID:   p.ID,
			Info: info,
		})
	case *SSHFxpMkdirPacket:
		// TODO FIXME: ignore flags field
		err := os.Mkdir(p.Path, 0755)
		return s.SendError(p, err)
	case *SSHFxpRmdirPacket:
		err := os.Remove(p.Path)
		return s.SendError(p, err)
	case *SSHFxpRemovePacket:
		err := os.Remove(p.Filename)
		return s.SendError(p, err)
	case *SSHFxpRenamePacket:
		err := os.Rename(p.Oldpath, p.Newpath)
		return s.SendError(p, err)
	case *SSHFxpSymlinkPacket:
		err := os.Symlink(p.Targetpath, p.Linkpath)
		return s.SendError(p, err)
	case *SSHFxpClosePacket:
		return s.SendError(p, be.CloseHandle(p.Handle))
	case *SSHFxpReadlinkPacket:
		f, err := os.Readlink(p.Path)
		if err != nil {
			return s.SendError(p, err)
		}

		return s.SendPacket(SSHFxpNamePacket{
			ID: p.ID,
			NameAttrs: []SSHFxpNameAttr{{
				Name:     f,
				LongName: f,
				Attrs:    emptyFileStat,
			}},
		})

	case *SSHFxpRealpathPacket:
		f, err := filepath.Abs(p.Path)
		if err != nil {
			return s.SendError(p, err)
		}
		f = cleanPath(f)
		return s.SendPacket(SSHFxpNamePacket{
			ID: p.ID,
			NameAttrs: []SSHFxpNameAttr{{
				Name:     f,
				LongName: f,
				Attrs:    emptyFileStat,
			}},
		})
	case *SSHFxpOpendirPacket:
		if stat, err := os.Stat(p.Path); err != nil {
			return s.SendError(p, err)
		} else if !stat.IsDir() {
			return s.SendError(p, &os.PathError{
				Path: p.Path, Err: syscall.ENOTDIR})
		}
		return be.RespondToOpenPacket(s, &SSHFxpOpenPacket{
			ID:     p.ID,
			Path:   p.Path,
			Pflags: ssh_FXF_READ,
		})
	case *SSHFxpReadPacket:
		f, ok := be.GetHandle(p.Handle)
		if !ok {
			return s.SendError(p, syscall.EBADF)
		}

		data := make([]byte, clamp(p.Len, s.maxTxPacket))
		n, err := f.ReadAt(data, int64(p.Offset))
		if err != nil && (err != io.EOF || n == 0) {
			return s.SendError(p, err)
		}
		return s.SendPacket(SSHFxpDataPacket{
			ID:     p.ID,
			Length: uint32(n),
			Data:   data[:n],
		})
	case *SSHFxpWritePacket:
		f, ok := be.GetHandle(p.Handle)
		if !ok {
			return s.SendError(p, syscall.EBADF)
		}

		_, err := f.WriteAt(p.Data, int64(p.Offset))
		return s.SendError(p, err)

	case *SSHFxpOpenPacket:
		return be.RespondToOpenPacket(s, p)

	case *SSHFxpReaddirPacket:
		return be.RespondToReaddirPacket(s, p)

	case *SSHFxpSetstatPacket:
		return be.RespondToSetstatPacket(s, p)

	case *SSHFxpFsetstatPacket:
		return be.RespondToFsetstatPacket(s, p)

	case *SSHFxpExtendedPacket:
		return be.RespondToExtendedPacket(s, p)

	case *SSHFxpExtendedPacketPosixRename:
		return be.RespondToExtendedPacketPosixRenamePacket(s, p)

	case *SSHFxpExtendedPacketStatVFS:
		return be.RespondToExtendedPacketStatVFS(s, p)

	default:
		return errors.Errorf("unexpected packet type %T", p)
	}
}

func (be *DefaultFSBackend) Close() {
	// close any still-open files
	for handle, file := range be.openFiles {
		be.log.Debugf("sftp server file with handle %q left open: %v\n", handle, file.Name())
		file.Close()
	}
}
