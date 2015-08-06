package sftp

// sftp server counterpart

import (
	"encoding"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"syscall"
)

type FileSystem interface {
	Lstat(name string) (os.FileInfo, error)
	Remove(name string) error
	Stat(name string) (os.FileInfo, error)
	Rename(oldpath, newpath string) error
}

type FileSystemOS interface {
	FileSystem
	OpenFile(name string, flag int, perm os.FileMode) (file *os.File, err error)
	Readlink(path string) (string, error)
	Realpath(path string) (string, error)
	Mkdir(name string, perm os.FileMode) error
}

type FileSystemSFTP interface {
	FileSystem
	OpenFile(path string, f int) (*File, error) // sftp package has a strange OpenFile method with no perm
	ReadLink(path string) (string, error)
	Mkdir(name string) error
}

// common subset of os.File and sftp.File
type svrFile interface {
	Chmod(mode os.FileMode) error
	Chown(uid, gid int) error
	Close() error
	Read(b []byte) (int, error)
	Seek(offset int64, whence int) (int64, error)
	Stat() (os.FileInfo, error)
	Truncate(size int64) error
	Write(b []byte) (int, error)
	// func (f *File) WriteTo(w io.Writer) (int64, error)	// not in os
	// func (f *File) ReadFrom(r io.Reader) (int64, error)	// not in os
}

type nativeFs struct {
}

func (nfs *nativeFs) Lstat(path string) (os.FileInfo, error)    { return os.Lstat(path) }
func (nfs *nativeFs) Stat(path string) (os.FileInfo, error)     { return os.Stat(path) }
func (nfs *nativeFs) Mkdir(path string, perm os.FileMode) error { return os.Mkdir(path, perm) }
func (nfs *nativeFs) Remove(path string) error                  { return os.Remove(path) }
func (nfs *nativeFs) Rename(oldpath, newpath string) error      { return os.Rename(oldpath, newpath) }
func (nfs *nativeFs) Readlink(path string) (string, error)      { return os.Readlink(path) }
func (nfs *nativeFs) Realpath(path string) (string, error) {
	f, err := filepath.Abs(path)
	return filepath.Clean(f), err
}
func (nfs *nativeFs) OpenFile(path string, flag int, perm os.FileMode) (file *os.File, err error) {
	return os.OpenFile(path, flag, perm)
}

var __typecheck_fsos FileSystemOS = &nativeFs{}
var __typecheck_sftpos FileSystemSFTP = &Client{}

type Server struct {
	in            io.Reader
	out           io.Writer
	debugStream   io.Writer
	debugLevel    int
	readOnly      bool
	rootDir       string
	lastId        uint32
	fs            FileSystem
	pktChan       chan serverRespondablePacket
	openFiles     map[string]svrFile
	openFilesLock *sync.RWMutex
	handleCount   int
	maxTxPacket   uint32
}

func (svr *Server) nextHandle(f svrFile) string {
	svr.openFilesLock.Lock()
	defer svr.openFilesLock.Unlock()
	svr.handleCount++
	handle := fmt.Sprintf("%d", svr.handleCount)
	svr.openFiles[handle] = f
	return handle
}

func (svr *Server) closeHandle(handle string) error {
	svr.openFilesLock.Lock()
	defer svr.openFilesLock.Unlock()
	if f, ok := svr.openFiles[handle]; ok {
		delete(svr.openFiles, handle)
		return f.Close()
	} else {
		return syscall.EBADF
	}
}

func (svr *Server) getHandle(handle string) (svrFile, bool) {
	svr.openFilesLock.RLock()
	defer svr.openFilesLock.RUnlock()
	f, ok := svr.openFiles[handle]
	return f, ok
}

type serverRespondablePacket interface {
	encoding.BinaryUnmarshaler
	respond(svr *Server) error
}

// Creates a new server instance around the provided streams.
// A subsequent call to Run() is required.
func NewServer(in io.Reader, out io.Writer, debugStream io.Writer, debugLevel int, readOnly bool, rootDir string) (*Server, error) {
	if rootDir == "" {
		if wd, err := os.Getwd(); err != nil {
			return nil, err
		} else {
			rootDir = wd
		}
	}
	return &Server{
		in:            in,
		out:           out,
		debugStream:   debugStream,
		debugLevel:    debugLevel,
		readOnly:      readOnly,
		rootDir:       rootDir,
		fs:            &nativeFs{},
		pktChan:       make(chan serverRespondablePacket, 4),
		openFiles:     map[string]svrFile{},
		openFilesLock: &sync.RWMutex{},
		maxTxPacket:   1 << 15,
	}, nil
}

// Unmarshal a single logical packet from the secure channel
func (svr *Server) rxPackets() error {
	defer close(svr.pktChan)

	for {
		pktType, pktBytes, err := recvPacket(svr.in)
		if err == io.EOF {
			fmt.Fprintf(svr.debugStream, "rxPackets loop done\n")
			return nil
		} else if err != nil {
			fmt.Fprintf(svr.debugStream, "recvPacket error: %v\n", err)
			return err
		}

		if pkt, err := svr.decodePacket(fxp(pktType), pktBytes); err != nil {
			fmt.Fprintf(svr.debugStream, "decodePacket error: %v\n", err)
			return err
		} else {
			svr.pktChan <- pkt
		}
	}
}

// Run this server until the streams stop or until the subsystem is stopped
func (svr *Server) Run() error {
	go svr.rxPackets()
	for pkt := range svr.pktChan {
		fmt.Fprintf(svr.debugStream, "pkt: %T %v\n", pkt, pkt)
		pkt.respond(svr)
	}
	fmt.Fprintf(svr.debugStream, "Run finished\n")
	return nil
}

func (svr *Server) decodePacket(pktType fxp, pktBytes []byte) (serverRespondablePacket, error) {
	//pktId, restBytes := unmarshalUint32(pktBytes[1:])
	var pkt serverRespondablePacket = nil
	switch pktType {
	case ssh_FXP_INIT:
		pkt = &sshFxInitPacket{}
	case ssh_FXP_LSTAT:
		pkt = &sshFxpLstatPacket{}
	case ssh_FXP_VERSION:
	case ssh_FXP_OPEN:
		pkt = &sshFxpOpenPacket{}
	case ssh_FXP_CLOSE:
		pkt = &sshFxpClosePacket{}
	case ssh_FXP_READ:
		pkt = &sshFxpReadPacket{}
	case ssh_FXP_WRITE:
		pkt = &sshFxpWritePacket{}
	case ssh_FXP_FSTAT:
		pkt = &sshFxpFstatPacket{}
	case ssh_FXP_SETSTAT:
	case ssh_FXP_FSETSTAT:
	case ssh_FXP_OPENDIR:
		pkt = &sshFxpOpendirPacket{}
	case ssh_FXP_READDIR:
		pkt = &sshFxpReaddirPacket{}
	case ssh_FXP_REMOVE:
		pkt = &sshFxpRemovePacket{}
	case ssh_FXP_MKDIR:
		pkt = &sshFxpMkdirPacket{}
	case ssh_FXP_RMDIR:
	case ssh_FXP_REALPATH:
		pkt = &sshFxpRealpathPacket{}
	case ssh_FXP_STAT:
		pkt = &sshFxpStatPacket{}
	case ssh_FXP_RENAME:
		pkt = &sshFxpRenamePacket{}
	case ssh_FXP_READLINK:
		pkt = &sshFxpReadlinkPacket{}
	case ssh_FXP_SYMLINK:
	case ssh_FXP_STATUS:
	case ssh_FXP_HANDLE:
	case ssh_FXP_DATA:
	case ssh_FXP_NAME:
	case ssh_FXP_ATTRS:
	case ssh_FXP_EXTENDED:
	case ssh_FXP_EXTENDED_REPLY:
	default:
	}
	if pkt == nil {
		return nil, fmt.Errorf("unhandled packet type: %s", pktType.String())
	}
	if err := pkt.UnmarshalBinary(pktBytes); err != nil {
		return nil, err
	}
	return pkt, nil
}

func (p sshFxInitPacket) respond(svr *Server) error {
	return svr.sendPacket(sshFxVersionPacket{sftpProtocolVersion, nil})
}

type sshFxpStatResponse struct {
	Id   uint32
	info os.FileInfo
}

func (p sshFxpStatResponse) MarshalBinary() ([]byte, error) {
	b := []byte{ssh_FXP_ATTRS}
	b = marshalUint32(b, p.Id)
	b = marshalFileInfo(b, p.info)
	return b, nil
}

func (p sshFxpLstatPacket) respond(svr *Server) error {
	// stat the requested file
	if info, err := svr.fs.Lstat(p.Path); err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		return svr.sendPacket(sshFxpStatResponse{p.Id, info})
	}
}

func (p sshFxpStatPacket) respond(svr *Server) error {
	// stat the requested file
	if info, err := svr.fs.Stat(p.Path); err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		return svr.sendPacket(sshFxpStatResponse{p.Id, info})
	}
}

func (p sshFxpFstatPacket) respond(svr *Server) error {
	if f, ok := svr.getHandle(p.Handle); !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	} else if osf, ok := f.(*os.File); ok {
		if info, err := osf.Stat(); err != nil {
			return svr.sendPacket(statusFromError(p.Id, err))
		} else {
			return svr.sendPacket(sshFxpStatResponse{p.Id, info})
		}
	} else {
		// server error...
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	}
}

func (p sshFxpMkdirPacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}
	// TODO FIXME: ignore flags field
	if fso, ok := svr.fs.(FileSystemOS); ok {
		err := fso.Mkdir(p.Path, 0755)
		return svr.sendPacket(statusFromError(p.Id, err))
	} else if sftpo, ok := svr.fs.(FileSystemSFTP); ok {
		err := sftpo.Mkdir(p.Path)
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		return svr.sendPacket(statusFromError(p.Id, fmt.Errorf("unknown filesystem backend")))
	}
}

func (p sshFxpRemovePacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}
	err := svr.fs.Remove(p.Filename)
	return svr.sendPacket(statusFromError(p.Id, err))
}

func (p sshFxpRenamePacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}
	err := svr.fs.Rename(p.Oldpath, p.Newpath)
	return svr.sendPacket(statusFromError(p.Id, err))
}

var emptyFileStat = []interface{}{uint32(0)}

func (p sshFxpReadlinkPacket) respond(svr *Server) error {
	if fso, ok := svr.fs.(FileSystemOS); ok {
		if f, err := fso.Readlink(p.Path); err != nil {
			return svr.sendPacket(statusFromError(p.Id, err))
		} else {
			return svr.sendPacket(sshFxpNamePacket{p.Id, []sshFxpNameAttr{sshFxpNameAttr{f, f, emptyFileStat}}})
		}
	} else if sftpo, ok := svr.fs.(FileSystemSFTP); ok {
		if f, err := sftpo.ReadLink(p.Path); err != nil {
			return svr.sendPacket(statusFromError(p.Id, err))
		} else {
			return svr.sendPacket(sshFxpNamePacket{p.Id, []sshFxpNameAttr{sshFxpNameAttr{f, f, emptyFileStat}}})
		}
	} else {
		return svr.sendPacket(statusFromError(p.Id, fmt.Errorf("unknown filesystem backend")))
	}
}

func (p sshFxpRealpathPacket) respond(svr *Server) error {
	if fso, ok := svr.fs.(FileSystemOS); ok {
		if f, err := fso.Realpath(p.Path); err != nil {
			return svr.sendPacket(statusFromError(p.Id, err))
		} else {
			return svr.sendPacket(sshFxpNamePacket{p.Id, []sshFxpNameAttr{sshFxpNameAttr{f, f, emptyFileStat}}})
		}
	} else {
		return svr.sendPacket(statusFromError(p.Id, fmt.Errorf("unknown filesystem backend")))
	}
}

func (p sshFxpOpendirPacket) respond(svr *Server) error {
	return sshFxpOpenPacket{p.Id, p.Path, ssh_FXF_READ, 0}.respond(svr)
}

func (p sshFxpOpenPacket) respond(svr *Server) error {
	osFlags := 0
	if p.Pflags&ssh_FXF_READ != 0 && p.Pflags&ssh_FXF_WRITE != 0 {
		if svr.readOnly {
			return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
		}
		osFlags |= os.O_RDWR
	} else if p.Pflags&ssh_FXF_WRITE != 0 {
		if svr.readOnly {
			return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
		}
		osFlags |= os.O_WRONLY
	} else if p.Pflags&ssh_FXF_READ != 0 {
		osFlags |= os.O_RDONLY
	} else {
		// how are they opening?
		return svr.sendPacket(statusFromError(p.Id, syscall.EINVAL))

	}

	if p.Pflags&ssh_FXF_APPEND != 0 {
		osFlags |= os.O_APPEND
	}
	if p.Pflags&ssh_FXF_CREAT != 0 {
		osFlags |= os.O_CREATE
	}
	if p.Pflags&ssh_FXF_TRUNC != 0 {
		osFlags |= os.O_TRUNC
	}
	if p.Pflags&ssh_FXF_EXCL != 0 {
		osFlags |= os.O_EXCL
	}

	if fso, ok := svr.fs.(FileSystemOS); ok {
		if f, err := fso.OpenFile(p.Path, osFlags, 0644); err != nil {
			return svr.sendPacket(statusFromError(p.Id, err))
		} else {
			handle := svr.nextHandle(f)
			return svr.sendPacket(sshFxpHandlePacket{p.Id, handle})
		}
	} else if sftpo, ok := svr.fs.(FileSystemSFTP); ok {
		if f, err := sftpo.OpenFile(p.Path, osFlags); err != nil {
			return svr.sendPacket(statusFromError(p.Id, err))
		} else {
			handle := svr.nextHandle(f)
			return svr.sendPacket(sshFxpHandlePacket{p.Id, handle})
		}
	} else {
		return svr.sendPacket(statusFromError(p.Id, fmt.Errorf("unknown filesystem backend")))
	}
}

func (p sshFxpClosePacket) respond(svr *Server) error {
	return svr.sendPacket(statusFromError(p.Id, svr.closeHandle(p.Handle)))
}

func (p sshFxpReadPacket) respond(svr *Server) error {
	if f, ok := svr.getHandle(p.Handle); !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	} else {
		if p.Len > svr.maxTxPacket {
			p.Len = svr.maxTxPacket
		}
		if osf, ok := f.(*os.File); ok {
			ret := sshFxpDataPacket{Id: p.Id, Length: p.Len, Data: make([]byte, p.Len)}
			if n, err := osf.ReadAt(ret.Data, int64(p.Offset)); err != nil && (err != io.EOF || n == 0) {
				return svr.sendPacket(statusFromError(p.Id, err))
			} else {
				ret.Length = uint32(n)
				return svr.sendPacket(ret)
			}
		} else {
			// server error...
			return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
		}
	}
}

func (p sshFxpWritePacket) respond(svr *Server) error {
	if svr.readOnly {
		// shouldn't really get here, the open should have failed
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}
	if f, ok := svr.getHandle(p.Handle); !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	} else if osf, ok := f.(*os.File); ok {
		_, err := osf.WriteAt(p.Data, int64(p.Offset))
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		// server error...
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	}
}

func (p sshFxpReaddirPacket) respond(svr *Server) error {
	if f, ok := svr.getHandle(p.Handle); !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	} else {
		dirname := ""
		dirents := []os.FileInfo{}
		var err error = nil

		if osf, ok := f.(*os.File); ok {
			dirname = osf.Name()
			dirents, err = osf.Readdir(128)
		} else {
			// server error...
			return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
		}

		if err != nil {
			return svr.sendPacket(statusFromError(p.Id, err))
		}

		ret := sshFxpNamePacket{p.Id, nil}
		for _, dirent := range dirents {
			ret.NameAttrs = append(ret.NameAttrs, sshFxpNameAttr{dirent.Name(), path.Join(dirname, dirent.Name()), []interface{}{dirent}})
		}
		return svr.sendPacket(ret)
	}
}

func errnoToSshErr(errno syscall.Errno) uint32 {
	if errno == 0 {
		return ssh_FX_OK
	} else if errno == syscall.ENOENT {
		return ssh_FX_NO_SUCH_FILE
	} else if errno == syscall.EPERM {
		return ssh_FX_PERMISSION_DENIED
	} else {
		return ssh_FX_FAILURE
	}

	return uint32(errno)
}

func statusFromError(id uint32, err error) sshFxpStatusPacket {
	ret := sshFxpStatusPacket{
		Id: id,
		StatusError: StatusError{
			// ssh_FX_OK                = 0
			// ssh_FX_EOF               = 1
			// ssh_FX_NO_SUCH_FILE      = 2 ENOENT
			// ssh_FX_PERMISSION_DENIED = 3
			// ssh_FX_FAILURE           = 4
			// ssh_FX_BAD_MESSAGE       = 5
			// ssh_FX_NO_CONNECTION     = 6
			// ssh_FX_CONNECTION_LOST   = 7
			// ssh_FX_OP_UNSUPPORTED    = 8
			Code: ssh_FX_OK,
			msg:  "",
			lang: "",
		},
	}
	if err != nil {
		debug("statusFromError: error is %T %#v", err, err)
		ret.StatusError.Code = ssh_FX_FAILURE
		ret.StatusError.msg = err.Error()
		if err == io.EOF {
			ret.StatusError.Code = ssh_FX_EOF
		} else if errno, ok := err.(syscall.Errno); ok {
			ret.StatusError.Code = errnoToSshErr(errno)
		} else if pathError, ok := err.(*os.PathError); ok {
			debug("statusFromError: error is %T %#v", pathError.Err, pathError.Err)
			if errno, ok := pathError.Err.(syscall.Errno); ok {
				ret.StatusError.Code = errnoToSshErr(errno)
			}
		}
	}
	return ret
}
