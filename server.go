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
	}

	return syscall.EBADF
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
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}

		rootDir = wd
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
		switch err {
		case nil:
			svr.pktChan <- rxPacket{fxp(pktType), pktBytes}
		case io.EOF:
			return nil
		default:
			fmt.Fprintf(svr.debugStream, "recvPacket error: %v\n", err)
			return err
		}
	}
}

// Up to N parallel servers
func (svr *Server) sftpServerWorker(doneChan chan error) {
	for pkt := range svr.pktChan {
		dPkt, err := svr.decodePacket(pkt.pktType, pkt.pktBytes)
		if err != nil {
			fmt.Fprintf(svr.debugStream, "decodePacket error: %v\n", err)
			doneChan <- err
			return
		}

		//fmt.Fprintf(svr.debugStream, "pkt: %T %v\n", pkt, pkt)
		dPkt.respond(svr)
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
	// close any still-open files
	for handle, file := range svr.openFiles {
		fmt.Fprintf(svr.debugStream, "sftp server file with handle '%v' left open: %v\n", handle, file.Name())
		file.Close()
	}
	return svr.out.Close()
}

func (svr *Server) decodePacket(pktType fxp, pktBytes []byte) (serverRespondablePacket, error) {
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
		return nil, fmt.Errorf("unhandled packet type: %s", pktType)
	}
	err := pkt.UnmarshalBinary(pktBytes)
	return pkt, err
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
	info, err := os.Lstat(p.Path)
	if err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	}

	return svr.sendPacket(sshFxpStatResponse{
		Id:   p.Id,
		info: info,
	})
}

func (p sshFxpStatPacket) respond(svr *Server) error {
	// stat the requested file
	info, err := os.Stat(p.Path)
	if err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	}

	return svr.sendPacket(sshFxpStatResponse{
		Id:   p.Id,
		info: info,
	})
}

func (p sshFxpFstatPacket) respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	}

	info, err := f.Stat()
	if err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	}

	return svr.sendPacket(sshFxpStatResponse{
		Id:   p.Id,
		info: info,
	})
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
	f, err := os.Readlink(p.Path)
	if err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	}

	return svr.sendPacket(sshFxpNamePacket{
		Id: p.Id,
		NameAttrs: []sshFxpNameAttr{{
			Name:     f,
			LongName: f,
			Attrs:    emptyFileStat,
		}},
	})
}

func (p sshFxpRealpathPacket) respond(svr *Server) error {
	f, err := filepath.Abs(p.Path)
	if err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	}

	f = filepath.Clean(f)

	return svr.sendPacket(sshFxpNamePacket{
		Id: p.Id,
		NameAttrs: []sshFxpNameAttr{{
			Name:     f,
			LongName: f,
			Attrs:    emptyFileStat,
		}},
	})
}

func (p sshFxpOpendirPacket) respond(svr *Server) error {
	return sshFxpOpenPacket{
		Id:     p.Id,
		Path:   p.Path,
		Pflags: ssh_FXF_READ,
	}.respond(svr)
}

func (p sshFxpOpenPacket) respond(svr *Server) error {
	var osFlags int
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

	f, err := os.OpenFile(p.Path, osFlags, 0644)
	if err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	}

	handle := svr.nextHandle(f)
	return svr.sendPacket(sshFxpHandlePacket{p.Id, handle})
}

func (p sshFxpClosePacket) respond(svr *Server) error {
	return svr.sendPacket(statusFromError(p.Id, svr.closeHandle(p.Handle)))
}

func (p sshFxpReadPacket) respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	}

	if p.Len > svr.maxTxPacket {
		p.Len = svr.maxTxPacket
	}
	ret := sshFxpDataPacket{
		Id:     p.Id,
		Length: p.Len,
		Data:   make([]byte, p.Len),
	}

	n, err := f.ReadAt(ret.Data, int64(p.Offset))
	if err != nil && (err != io.EOF || n == 0) {
		return svr.sendPacket(statusFromError(p.Id, err))
	}

	ret.Length = uint32(n)
	return svr.sendPacket(ret)
}

func (p sshFxpWritePacket) respond(svr *Server) error {
	if svr.readOnly {
		// shouldn't really get here, the open should have failed
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}

	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	}

	_, err := f.WriteAt(p.Data, int64(p.Offset))
	return svr.sendPacket(statusFromError(p.Id, err))
}

func (p sshFxpReaddirPacket) respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	}

	dirname := f.Name()
	dirents, err := f.Readdir(128)
	if err != nil {
		return svr.sendPacket(statusFromError(p.Id, err))
	}

	ret := sshFxpNamePacket{Id: p.Id}
	for _, dirent := range dirents {
		ret.NameAttrs = append(ret.NameAttrs, sshFxpNameAttr{
			Name:     dirent.Name(),
			LongName: runLs(dirname, dirent),
			Attrs:    []interface{}{dirent},
		})
	}
	return svr.sendPacket(ret)
}

func (p sshFxpSetstatPacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}

	// additional unmarshalling is required for each possibility here
	b := p.Attrs.([]byte)
	var err error

	debug("setstat name \"%s\"", p.Path)
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
		} else if gid, b, err = unmarshalUint32Safe(b); err != nil {
		} else {
			err = os.Chown(p.Path, int(uid), int(gid))
		}
	}

	return svr.sendPacket(statusFromError(p.Id, err))
}

func (p sshFxpFsetstatPacket) respond(svr *Server) error {
	if svr.readOnly {
		return svr.sendPacket(statusFromError(p.Id, syscall.EPERM))
	}

	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.sendPacket(statusFromError(p.Id, syscall.EBADF))
	}

	// additional unmarshalling is required for each possibility here
	b := p.Attrs.([]byte)
	var err error

	debug("fsetstat name \"%s\"", f.Name())
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
		} else if gid, b, err = unmarshalUint32Safe(b); err != nil {
		} else {
			err = f.Chown(int(uid), int(gid))
		}
	}

	return svr.sendPacket(statusFromError(p.Id, err))
}

func errnoToSshErr(errno syscall.Errno) uint32 {
	switch errno {
	case 0:
		return ssh_FX_OK
	case syscall.ENOENT:
		return ssh_FX_NO_SUCH_FILE
	case syscall.EPERM:
		return ssh_FX_PERMISSION_DENIED
	}

	return ssh_FX_FAILURE
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
