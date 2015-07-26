package sftp

// sftp server counterpart

import (
	"encoding"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
)

type FileSystem interface {
	Lstat(p string) (os.FileInfo, error)
	Mkdir(name string, perm os.FileMode) error
}

type FileSystemOpen interface {
	FileSystem
	OpenFile(name string, flag int, perm os.FileMode) (file *os.File, err error)
}

type FileSystemSFTPOpen interface {
	FileSystem
	OpenFile(path string, f int) (*File, error) // sftp package has a strange OpenFile method with no perm
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

func (nfs *nativeFs) Lstat(p string) (os.FileInfo, error)       { return os.Lstat(p) }
func (nfs *nativeFs) Mkdir(name string, perm os.FileMode) error { return os.Mkdir(name, perm) }
func (nfs *nativeFs) OpenFile(name string, flag int, perm os.FileMode) (file *os.File, err error) {
	return os.OpenFile(name, flag, perm)
}

type Server struct {
	in            io.Reader
	out           io.Writer
	rootDir       string
	lastId        uint32
	fs            FileSystem
	pktChan       chan serverRespondablePacket
	openFiles     map[string]svrFile
	openFilesLock *sync.RWMutex
	handleCount   int
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

type serverRespondablePacket interface {
	encoding.BinaryUnmarshaler
	respond(svr *Server) error
}

// Creates a new server instance around the provided streams.
// A subsequent call to Run() is required.
func NewServer(in io.Reader, out io.Writer, rootDir string) (*Server, error) {
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
		rootDir:       rootDir,
		fs:            &nativeFs{},
		pktChan:       make(chan serverRespondablePacket, 4),
		openFiles:     map[string]svrFile{},
		openFilesLock: &sync.RWMutex{},
	}, nil
}

// Unmarshal a single logical packet from the secure channel
func (svr *Server) rxPackets() error {
	defer close(svr.pktChan)

	for {
		pktType, pktBytes, err := recvPacket(svr.in)
		if err == io.EOF {
			return nil
		} else if err != nil {
			fmt.Fprintf(os.Stderr, "recvPacket error: %v\n", err)
			return err
		}

		if pkt, err := svr.decodePacket(fxp(pktType), pktBytes); err != nil {
			fmt.Fprintf(os.Stderr, "decodePacket error: %v\n", err)
			return err
		} else {
			svr.pktChan <- pkt
		}
	}
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
	case ssh_FXP_WRITE:
	case ssh_FXP_FSTAT:
	case ssh_FXP_SETSTAT:
	case ssh_FXP_FSETSTAT:
	case ssh_FXP_OPENDIR:
	case ssh_FXP_READDIR:
	case ssh_FXP_REMOVE:
	case ssh_FXP_MKDIR:
		pkt = &sshFxpMkdirPacket{}
	case ssh_FXP_RMDIR:
	case ssh_FXP_REALPATH:
	case ssh_FXP_STAT:
	case ssh_FXP_RENAME:
	case ssh_FXP_READLINK:
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

// Run this server until the streams stop or until the subsystem is stopped
func (svr *Server) Run() error {
	go svr.rxPackets()
	for pkt := range svr.pktChan {
		fmt.Fprintf(os.Stderr, "pkt: %T %v\n", pkt, pkt)
		pkt.respond(svr)
	}
	return nil
}

func (p sshFxInitPacket) respond(svr *Server) error {
	return svr.sendPacket(sshFxVersionPacket{sftpProtocolVersion, nil})
}

type sshFxpLstatReponse struct {
	Id   uint32
	info os.FileInfo
}

func (p sshFxpLstatReponse) MarshalBinary() ([]byte, error) {
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
		return svr.sendPacket(sshFxpLstatReponse{p.Id, info})
	}
}

func (p sshFxpMkdirPacket) respond(svr *Server) error {
	// ignore flags field
	err := svr.fs.Mkdir(p.Path, 0755)
	return svr.sendPacket(statusFromError(p.Id, err))
}

func (p sshFxpOpenPacket) respond(svr *Server) error {
	osFlags := 0
	if p.Pflags&ssh_FXF_READ != 0 && p.Pflags&ssh_FXF_WRITE != 0 {
		osFlags |= os.O_RDWR
	} else if p.Pflags&ssh_FXF_READ != 0 {
		osFlags |= os.O_RDONLY
	} else if p.Pflags&ssh_FXF_WRITE != 0 {
		osFlags |= os.O_WRONLY
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

	if fso, ok := svr.fs.(FileSystemOpen); ok {
		if f, err := fso.OpenFile(p.Path, osFlags, 0644); err != nil {
			return svr.sendPacket(statusFromError(p.Id, err))
		} else {
			handle := svr.nextHandle(f)
			return svr.sendPacket(sshFxpHandlePacket{p.Id, handle})
		}
	} else if sftpo, ok := svr.fs.(FileSystemSFTPOpen); ok {
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
		}
		if pathError, ok := err.(*os.PathError); ok {
			debug("statusFromError: error is %T %#v", pathError.Err, pathError.Err)
			if errno, ok := pathError.Err.(syscall.Errno); ok {
				if errno == 0 {
					ret.StatusError.Code = ssh_FX_OK
				} else if errno == syscall.ENOENT {
					ret.StatusError.Code = ssh_FX_NO_SUCH_FILE
				} else if errno == syscall.EPERM {
					ret.StatusError.Code = ssh_FX_PERMISSION_DENIED
				} else {
					ret.StatusError.Code = ssh_FX_FAILURE
				}

				ret.StatusError.Code = uint32(errno)
			}
		}
	}
	return ret
}
