package sftp

// sftp server counterpart

import (
	"encoding"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	sftpServerWorkerCount = 8
)

// Server is an SSH File Transfer Protocol (sftp) server.
// This is intended to provide the sftp subsystem to an ssh server daemon.
// This implementation currently supports most of sftp server protocol version 3,
// as specified at http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02
type Server struct {
	in            io.Reader
	out           io.WriteCloser
	outMutex      *sync.Mutex
	debugStream   io.Writer
	debugLevel    int
	readOnly      bool
	rootDir       string
	lastId        uint32
	pktChan       chan rxPacket
	openFiles     map[string]*os.File
	openFilesLock *sync.RWMutex
	handleCount   int
	maxTxPacket   uint32
	workerCount   int
}

func (svr *Server) nextHandle(f *os.File) string {
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

func (svr *Server) getHandle(handle string) (*os.File, bool) {
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
// Various debug output will be written to debugStream, with verbosity set by debugLevel
// A subsequent call to Serve() is required.
func NewServer(in io.Reader, out io.WriteCloser, debugStream io.Writer, debugLevel int, readOnly bool, rootDir string) (*Server, error) {
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
		outMutex:      &sync.Mutex{},
		debugStream:   debugStream,
		debugLevel:    debugLevel,
		readOnly:      readOnly,
		rootDir:       rootDir,
		pktChan:       make(chan rxPacket, sftpServerWorkerCount),
		openFiles:     map[string]*os.File{},
		openFilesLock: &sync.RWMutex{},
		maxTxPacket:   1 << 15,
		workerCount:   sftpServerWorkerCount,
	}, nil
}

type rxPacket struct {
	pktType  fxp
	pktBytes []byte
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

		svr.pktChan <- rxPacket{fxp(pktType), pktBytes}
	}
}

// Up to N parallel servers
func (svr *Server) sftpServerWorker(doneChan chan error) {
	for pkt := range svr.pktChan {
		if pkt, err := svr.decodePacket(pkt.pktType, pkt.pktBytes); err != nil {
			fmt.Fprintf(svr.debugStream, "decodePacket error: %v\n", err)
			doneChan <- err
			return
		} else {
			//fmt.Fprintf(svr.debugStream, "pkt: %T %v\n", pkt, pkt)
			pkt.respond(svr)
		}
	}
	doneChan <- nil
}

// Run this server until the streams stop or until the subsystem is stopped
func (svr *Server) Serve() error {
	go svr.rxPackets()
	doneChan := make(chan error)
	for i := 0; i < svr.workerCount; i++ {
		go svr.sftpServerWorker(doneChan)
	}
	for i := 0; i < svr.workerCount; i++ {
		if err := <-doneChan; err != nil {
			// abort early and shut down the session on un-decodable packets
			break
		}
	}
	fmt.Fprintf(svr.debugStream, "sftp server run finished\n")
	// close any still-open files
	for handle, file := range svr.openFiles {
		fmt.Fprintf(svr.debugStream, "sftp server file with handle '%v' left open: %v\n", handle, file.Name())
		file.Close()
	}
	return svr.out.Close()
}

func (svr *Server) decodePacket(pktType fxp, pktBytes []byte) (serverRespondablePacket, error) {
	//pktId, restBytes := unmarshalUint32(pktBytes[1:])
	var pkt serverRespondablePacket = nil
	switch pktType {
	case ssh_FXP_INIT:
		pkt = &sshFxInitPacket{}
	case ssh_FXP_LSTAT:
		pkt = &sshFxpLstatPacket{}
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
		pkt = &sshFxpSetstatPacket{}
	case ssh_FXP_FSETSTAT:
		pkt = &sshFxpFsetstatPacket{}
	case ssh_FXP_OPENDIR:
		pkt = &sshFxpOpendirPacket{}
	case ssh_FXP_READDIR:
		pkt = &sshFxpReaddirPacket{}
	case ssh_FXP_REMOVE:
		pkt = &sshFxpRemovePacket{}
	case ssh_FXP_MKDIR:
		pkt = &sshFxpMkdirPacket{}
	case ssh_FXP_RMDIR:
		pkt = &sshFxpRmdirPacket{}
	case ssh_FXP_REALPATH:
		pkt = &sshFxpRealpathPacket{}
	case ssh_FXP_STAT:
		pkt = &sshFxpStatPacket{}
	case ssh_FXP_RENAME:
		pkt = &sshFxpRenamePacket{}
	case ssh_FXP_READLINK:
		pkt = &sshFxpReadlinkPacket{}
	case ssh_FXP_SYMLINK:
		pkt = &sshFxpSymlinkPacket{}
	default:
		return nil, fmt.Errorf("unhandled packet type: %s", pktType.String())
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
	if info, err := os.Lstat(p.Path); err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		return svr.sendPacket(sshFxpStatResponse{p.Id, info})
	}
}

func (p sshFxpStatPacket) respond(svr *Server) error {
	// stat the requested file
	if info, err := os.Stat(p.Path); err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		return svr.sendPacket(sshFxpStatResponse{p.Id, info})
	}
}

func (p sshFxpFstatPacket) respond(svr *Server) error {
	if f, ok := svr.getHandle(p.Handle); !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	} else if info, err := f.Stat(); err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		return svr.sendPacket(sshFxpStatResponse{p.Id, info})
	}
}

func (p sshFxpMkdirPacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}
	// TODO FIXME: ignore flags field
	err := os.Mkdir(p.Path, 0755)
	return svr.sendPacket(statusFromError(p.Id, err))
}

func (p sshFxpRmdirPacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}
	err := os.Remove(p.Path)
	return svr.sendPacket(statusFromError(p.Id, err))
}

func (p sshFxpRemovePacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}
	err := os.Remove(p.Filename)
	return svr.sendPacket(statusFromError(p.Id, err))
}

func (p sshFxpRenamePacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}
	err := os.Rename(p.Oldpath, p.Newpath)
	return svr.sendPacket(statusFromError(p.Id, err))
}

func (p sshFxpSymlinkPacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}
	err := os.Symlink(p.Targetpath, p.Linkpath)
	return svr.sendPacket(statusFromError(p.Id, err))
}

var emptyFileStat = []interface{}{uint32(0)}

func (p sshFxpReadlinkPacket) respond(svr *Server) error {
	if f, err := os.Readlink(p.Path); err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		return svr.sendPacket(sshFxpNamePacket{p.Id, []sshFxpNameAttr{sshFxpNameAttr{f, f, emptyFileStat}}})
	}
}

func (p sshFxpRealpathPacket) respond(svr *Server) error {
	if f, err := filepath.Abs(p.Path); err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		f = filepath.Clean(f)
		return svr.sendPacket(sshFxpNamePacket{p.Id, []sshFxpNameAttr{sshFxpNameAttr{f, f, emptyFileStat}}})
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

	if f, err := os.OpenFile(p.Path, osFlags, 0644); err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	} else {
		handle := svr.nextHandle(f)
		return svr.sendPacket(sshFxpHandlePacket{p.Id, handle})
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
		ret := sshFxpDataPacket{Id: p.Id, Length: p.Len, Data: make([]byte, p.Len)}
		if n, err := f.ReadAt(ret.Data, int64(p.Offset)); err != nil && (err != io.EOF || n == 0) {
			return svr.sendPacket(statusFromError(p.Id, err))
		} else {
			ret.Length = uint32(n)
			return svr.sendPacket(ret)
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
	} else {
		_, err := f.WriteAt(p.Data, int64(p.Offset))
		return svr.sendPacket(statusFromError(p.Id, err))
	}
}

func (p sshFxpReaddirPacket) respond(svr *Server) error {
	if f, ok := svr.getHandle(p.Handle); !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	} else {
		dirname := ""
		dirents := []os.FileInfo{}
		var err error = nil

		dirname = f.Name()
		dirents, err = f.Readdir(128)
		if err != nil {
			return svr.sendPacket(statusFromError(p.Id, err))
		}

		ret := sshFxpNamePacket{p.Id, nil}
		for _, dirent := range dirents {
			ret.NameAttrs = append(ret.NameAttrs, sshFxpNameAttr{
				dirent.Name(),
				runLs(dirname, dirent),
				[]interface{}{dirent},
			})
		}
		return svr.sendPacket(ret)
	}
}

func (p sshFxpSetstatPacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	} else {
		// additional unmarshalling is required for each possibility here
		b := p.Attrs.([]byte)
		var err error = nil

		debug("setstat name \"%s\"", p.Path)
		if (p.Flags & ssh_FILEXFER_ATTR_SIZE) != 0 {
			var size uint64 = 0
			if size, b, err = unmarshalUint64Safe(b); err == nil {
				err = os.Truncate(p.Path, int64(size))
			}
		}
		if (p.Flags & ssh_FILEXFER_ATTR_PERMISSIONS) != 0 {
			var mode uint32 = 0
			if mode, b, err = unmarshalUint32Safe(b); err == nil {
				err = os.Chmod(p.Path, os.FileMode(mode))
			}
		}
		if (p.Flags & ssh_FILEXFER_ATTR_ACMODTIME) != 0 {
			var atime uint32 = 0
			var mtime uint32 = 0
			if atime, b, err = unmarshalUint32Safe(b); err != nil {
			} else if mtime, b, err = unmarshalUint32Safe(b); err != nil {
			} else {
				atimeT := time.Unix(int64(atime), 0)
				mtimeT := time.Unix(int64(mtime), 0)
				err = os.Chtimes(p.Path, atimeT, mtimeT)
			}
		}
		if (p.Flags & ssh_FILEXFER_ATTR_UIDGID) != 0 {
			var uid uint32 = 0
			var gid uint32 = 0
			if uid, b, err = unmarshalUint32Safe(b); err != nil {
			} else if gid, b, err = unmarshalUint32Safe(b); err != nil {
			} else {
				err = os.Chown(p.Path, int(uid), int(gid))
			}
		}

		return svr.sendPacket(statusFromError(p.Id, err))
	}
}

func (p sshFxpFsetstatPacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	} else if f, ok := svr.getHandle(p.Handle); !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	} else {
		// additional unmarshalling is required for each possibility here
		b := p.Attrs.([]byte)
		var err error = nil

		debug("fsetstat name \"%s\"", f.Name())
		if (p.Flags & ssh_FILEXFER_ATTR_SIZE) != 0 {
			var size uint64 = 0
			if size, b, err = unmarshalUint64Safe(b); err == nil {
				err = f.Truncate(int64(size))
			}
		}
		if (p.Flags & ssh_FILEXFER_ATTR_PERMISSIONS) != 0 {
			var mode uint32 = 0
			if mode, b, err = unmarshalUint32Safe(b); err == nil {
				err = f.Chmod(os.FileMode(mode))
			}
		}
		if (p.Flags & ssh_FILEXFER_ATTR_ACMODTIME) != 0 {
			var atime uint32 = 0
			var mtime uint32 = 0
			if atime, b, err = unmarshalUint32Safe(b); err != nil {
			} else if mtime, b, err = unmarshalUint32Safe(b); err != nil {
			} else {
				atimeT := time.Unix(int64(atime), 0)
				mtimeT := time.Unix(int64(mtime), 0)
				err = os.Chtimes(f.Name(), atimeT, mtimeT)
			}
		}
		if (p.Flags & ssh_FILEXFER_ATTR_UIDGID) != 0 {
			var uid uint32 = 0
			var gid uint32 = 0
			if uid, b, err = unmarshalUint32Safe(b); err != nil {
			} else if gid, b, err = unmarshalUint32Safe(b); err != nil {
			} else {
				err = f.Chown(int(uid), int(gid))
			}
		}

		return svr.sendPacket(statusFromError(p.Id, err))
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
