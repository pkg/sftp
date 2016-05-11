package sftp

// sftp server counterpart

import (
	"encoding"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
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
	readOnly      bool
	lastID        uint32
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
	handle := strconv.Itoa(svr.handleCount)
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

type ServerRespondablePacket interface {
	encoding.BinaryUnmarshaler
	id() uint32
	Respond(svr *Server) error
	readonly() bool
}

// NewServer creates a new Server instance around the provided streams, serving
// content from the root of the filesystem.  Optionally, ServerOption
// functions may be specified to further configure the Server.
//
// A subsequent call to Serve() is required to begin serving files over SFTP.
func NewServer(in io.Reader, out io.WriteCloser, options ...ServerOption) (*Server, error) {
	s := &Server{
		in:            in,
		out:           out,
		outMutex:      &sync.Mutex{},
		debugStream:   ioutil.Discard,
		pktChan:       make(chan rxPacket, sftpServerWorkerCount),
		openFiles:     map[string]*os.File{},
		openFilesLock: &sync.RWMutex{},
		maxTxPacket:   1 << 15,
		workerCount:   sftpServerWorkerCount,
	}

	for _, o := range options {
		if err := o(s); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// A ServerOption is a function which applies configuration to a Server.
type ServerOption func(*Server) error

// WithDebug enables Server debugging output to the supplied io.Writer.
func WithDebug(w io.Writer) ServerOption {
	return func(s *Server) error {
		s.debugStream = w
		return nil
	}
}

// ReadOnly configures a Server to serve files in read-only mode.
func ReadOnly() ServerOption {
	return func(s *Server) error {
		s.readOnly = true
		return nil
	}
}

type rxPacket struct {
	pktType  Fxpkt
	pktBytes []byte
}

// Unmarshal a single logical packet from the secure channel
func (svr *Server) rxPackets() error {
	defer close(svr.pktChan)

	for {
		pktType, pktBytes, err := recvPacket(svr.in)
		switch err {
		case nil:
			svr.pktChan <- rxPacket{Fxpkt(pktType), pktBytes}
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

		// If server is operating read-only and a write operation is requested,
		// return permission denied
		if !dPkt.readonly() && svr.readOnly {
			_ = svr.SendPacket(StatusFromError(dPkt.id(), syscall.EPERM))
			continue
		}

		_ = dPkt.Respond(svr)
	}
	doneChan <- nil
}

// Serve serves SFTP connections until the streams stop or the SFTP subsystem
// is stopped.
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

func (svr *Server) decodePacket(pktType Fxpkt, pktBytes []byte) (ServerRespondablePacket, error) {
	var pkt ServerRespondablePacket
	switch pktType {
	case SSH_FXP_INIT:
		pkt = &SshFxInitPacket{}
	case SSH_FXP_LSTAT:
		pkt = &SshFxpLstatPacket{}
	case SSH_FXP_OPEN:
		pkt = &SshFxpOpenPacket{}
	case SSH_FXP_CLOSE:
		pkt = &SshFxpClosePacket{}
	case SSH_FXP_READ:
		pkt = &SshFxpReadPacket{}
	case SSH_FXP_WRITE:
		pkt = &SshFxpWritePacket{}
	case SSH_FXP_FSTAT:
		pkt = &SshFxpFstatPacket{}
	case SSH_FXP_SETSTAT:
		pkt = &SshFxpSetstatPacket{}
	case SSH_FXP_FSETSTAT:
		pkt = &SshFxpFsetstatPacket{}
	case SSH_FXP_OPENDIR:
		pkt = &SshFxpOpendirPacket{}
	case SSH_FXP_READDIR:
		pkt = &SshFxpReaddirPacket{}
	case SSH_FXP_REMOVE:
		pkt = &SshFxpRemovePacket{}
	case SSH_FXP_MKDIR:
		pkt = &SshFxpMkdirPacket{}
	case SSH_FXP_RMDIR:
		pkt = &SshFxpRmdirPacket{}
	case SSH_FXP_REALPATH:
		pkt = &SshFxpRealpathPacket{}
	case SSH_FXP_STAT:
		pkt = &SshFxpStatPacket{}
	case SSH_FXP_RENAME:
		pkt = &SshFxpRenamePacket{}
	case SSH_FXP_READLINK:
		pkt = &SshFxpReadlinkPacket{}
	case SSH_FXP_SYMLINK:
		pkt = &SshFxpSymlinkPacket{}
	default:
		return nil, fmt.Errorf("unhandled packet type: %s", pktType)
	}
	err := pkt.UnmarshalBinary(pktBytes)
	return pkt, err
}

func (p SshFxInitPacket) Respond(svr *Server) error {
	return svr.SendPacket(SshFxVersionPacket{sftpProtocolVersion, nil})
}

// The init packet has no ID, so we just return a zero-value ID
func (p SshFxInitPacket) id() uint32     { return 0 }
func (p SshFxInitPacket) readonly() bool { return true }

type SshFxpStatResponse struct {
	ID   uint32
	info os.FileInfo
}

func (p SshFxpStatResponse) MarshalBinary() ([]byte, error) {
	b := []byte{SSH_FXP_ATTRS}
	b = marshalUint32(b, p.ID)
	b = marshalFileInfo(b, p.info)
	return b, nil
}

func (p SshFxpLstatPacket) readonly() bool { return true }

func (p SshFxpLstatPacket) Respond(svr *Server) error {
	// stat the requested file
	info, err := os.Lstat(p.Path)
	if err != nil {
		return svr.SendPacket(StatusFromError(p.ID, err))
	}

	return svr.SendPacket(SshFxpStatResponse{
		ID:   p.ID,
		info: info,
	})
}

func (p SshFxpStatPacket) readonly() bool { return true }

func (p SshFxpStatPacket) Respond(svr *Server) error {
	// stat the requested file
	info, err := os.Stat(p.Path)
	if err != nil {
		return svr.SendPacket(StatusFromError(p.ID, err))
	}

	return svr.SendPacket(SshFxpStatResponse{
		ID:   p.ID,
		info: info,
	})
}

func (p SshFxpFstatPacket) readonly() bool { return true }

func (p SshFxpFstatPacket) Respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.SendPacket(StatusFromError(p.ID, syscall.EBADF))
	}

	info, err := f.Stat()
	if err != nil {
		return svr.SendPacket(StatusFromError(p.ID, err))
	}

	return svr.SendPacket(SshFxpStatResponse{
		ID:   p.ID,
		info: info,
	})
}

func (p SshFxpMkdirPacket) readonly() bool { return false }

func (p SshFxpMkdirPacket) Respond(svr *Server) error {
	// TODO FIXME: ignore flags field
	err := os.Mkdir(p.Path, 0755)
	return svr.SendPacket(StatusFromError(p.ID, err))
}

func (p SshFxpRmdirPacket) readonly() bool { return false }

func (p SshFxpRmdirPacket) Respond(svr *Server) error {
	err := os.Remove(p.Path)
	return svr.SendPacket(StatusFromError(p.ID, err))
}

func (p SshFxpRemovePacket) readonly() bool { return false }

func (p SshFxpRemovePacket) Respond(svr *Server) error {
	err := os.Remove(p.Filename)
	return svr.SendPacket(StatusFromError(p.ID, err))
}

func (p SshFxpRenamePacket) readonly() bool { return false }

func (p SshFxpRenamePacket) Respond(svr *Server) error {
	err := os.Rename(p.Oldpath, p.Newpath)
	return svr.SendPacket(StatusFromError(p.ID, err))
}

func (p SshFxpSymlinkPacket) readonly() bool { return false }

func (p SshFxpSymlinkPacket) Respond(svr *Server) error {
	err := os.Symlink(p.Targetpath, p.Linkpath)
	return svr.SendPacket(StatusFromError(p.ID, err))
}

var emptyFileStat = []interface{}{uint32(0)}

func (p SshFxpReadlinkPacket) readonly() bool { return true }

func (p SshFxpReadlinkPacket) Respond(svr *Server) error {
	f, err := os.Readlink(p.Path)
	if err != nil {
		return svr.SendPacket(StatusFromError(p.ID, err))
	}

	return svr.SendPacket(SshFxpNamePacket{
		ID: p.ID,
		NameAttrs: []SshFxpNameAttr{{
			Name:     f,
			LongName: f,
			Attrs:    emptyFileStat,
		}},
	})
}

func (p SshFxpRealpathPacket) readonly() bool { return true }

func (p SshFxpRealpathPacket) Respond(svr *Server) error {
	f, err := filepath.Abs(p.Path)
	if err != nil {
		return svr.SendPacket(StatusFromError(p.ID, err))
	}

	f = filepath.Clean(f)

	return svr.SendPacket(SshFxpNamePacket{
		ID: p.ID,
		NameAttrs: []SshFxpNameAttr{{
			Name:     f,
			LongName: f,
			Attrs:    emptyFileStat,
		}},
	})
}

func (p SshFxpOpendirPacket) readonly() bool { return true }

func (p SshFxpOpendirPacket) Respond(svr *Server) error {
	return SshFxpOpenPacket{
		ID:     p.ID,
		Path:   p.Path,
		Pflags: SSH_FXF_READ,
	}.Respond(svr)
}

func (p SshFxpOpenPacket) readonly() bool {
	return !p.hasPflags(SSH_FXF_WRITE)
}

func (p SshFxpOpenPacket) hasPflags(flags ...uint32) bool {
	for _, f := range flags {
		if p.Pflags&f == 0 {
			return false
		}
	}

	return true
}

func (p SshFxpOpenPacket) Respond(svr *Server) error {
	var osFlags int
	if p.hasPflags(SSH_FXF_READ, SSH_FXF_WRITE) {
		osFlags |= os.O_RDWR
	} else if p.hasPflags(SSH_FXF_WRITE) {
		osFlags |= os.O_WRONLY
	} else if p.hasPflags(SSH_FXF_READ) {
		osFlags |= os.O_RDONLY
	} else {
		// how are they opening?
		return svr.SendPacket(StatusFromError(p.ID, syscall.EINVAL))
	}

	if p.hasPflags(SSH_FXF_APPEND) {
		osFlags |= os.O_APPEND
	}
	if p.hasPflags(SSH_FXF_CREAT) {
		osFlags |= os.O_CREATE
	}
	if p.hasPflags(SSH_FXF_TRUNC) {
		osFlags |= os.O_TRUNC
	}
	if p.hasPflags(SSH_FXF_EXCL) {
		osFlags |= os.O_EXCL
	}

	f, err := os.OpenFile(p.Path, osFlags, 0644)
	if err != nil {
		return svr.SendPacket(StatusFromError(p.ID, err))
	}

	handle := svr.nextHandle(f)
	return svr.SendPacket(SshFxpHandlePacket{p.ID, handle})
}

func (p SshFxpClosePacket) readonly() bool { return true }

func (p SshFxpClosePacket) Respond(svr *Server) error {
	return svr.SendPacket(StatusFromError(p.ID, svr.closeHandle(p.Handle)))
}

func (p SshFxpReadPacket) readonly() bool { return true }

func (p SshFxpReadPacket) Respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.SendPacket(StatusFromError(p.ID, syscall.EBADF))
	}

	if p.Len > svr.maxTxPacket {
		p.Len = svr.maxTxPacket
	}
	ret := SshFxpDataPacket{
		ID:     p.ID,
		Length: p.Len,
		Data:   make([]byte, p.Len),
	}

	n, err := f.ReadAt(ret.Data, int64(p.Offset))
	if err != nil && (err != io.EOF || n == 0) {
		return svr.SendPacket(StatusFromError(p.ID, err))
	}

	ret.Length = uint32(n)
	return svr.SendPacket(ret)
}

func (p SshFxpWritePacket) readonly() bool { return false }

func (p SshFxpWritePacket) Respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.SendPacket(StatusFromError(p.ID, syscall.EBADF))
	}

	_, err := f.WriteAt(p.Data, int64(p.Offset))
	return svr.SendPacket(StatusFromError(p.ID, err))
}

func (p SshFxpReaddirPacket) readonly() bool { return true }

func (p SshFxpReaddirPacket) Respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.SendPacket(StatusFromError(p.ID, syscall.EBADF))
	}

	dirname := f.Name()
	dirents, err := f.Readdir(128)
	if err != nil {
		return svr.SendPacket(StatusFromError(p.ID, err))
	}

	ret := SshFxpNamePacket{ID: p.ID}
	for _, dirent := range dirents {
		ret.NameAttrs = append(ret.NameAttrs, SshFxpNameAttr{
			Name:     dirent.Name(),
			LongName: runLs(dirname, dirent),
			Attrs:    []interface{}{dirent},
		})
	}
	return svr.SendPacket(ret)
}

func (p SshFxpSetstatPacket) readonly() bool { return false }

func (p SshFxpSetstatPacket) Respond(svr *Server) error {
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

	return svr.SendPacket(StatusFromError(p.ID, err))
}

func (p SshFxpFsetstatPacket) readonly() bool { return false }

func (p SshFxpFsetstatPacket) Respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok {
		return svr.SendPacket(StatusFromError(p.ID, syscall.EBADF))
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

	return svr.SendPacket(StatusFromError(p.ID, err))
}

// translateErrno translates a syscall error number to a SFTP error code.
func translateErrno(errno syscall.Errno) uint32 {
	switch errno {
	case 0:
		return SSH_FX_OK
	case syscall.ENOENT:
		return SSH_FX_NO_SUCH_FILE
	case syscall.EPERM:
		return SSH_FX_PERMISSION_DENIED
	}

	return SSH_FX_FAILURE
}

func StatusFromError(id uint32, err error) SshFxpStatusPacket {
	ret := SshFxpStatusPacket{
		ID: id,
		StatusError: StatusError{
			// SSH_FX_OK                = 0
			// SSH_FX_EOF               = 1
			// SSH_FX_NO_SUCH_FILE      = 2 ENOENT
			// SSH_FX_PERMISSION_DENIED = 3
			// SSH_FX_FAILURE           = 4
			// SSH_FX_BAD_MESSAGE       = 5
			// SSH_FX_NO_CONNECTION     = 6
			// SSH_FX_CONNECTION_LOST   = 7
			// SSH_FX_OP_UNSUPPORTED    = 8
			Code: SSH_FX_OK,
		},
	}
	if err != nil {
		debug("StatusFromError: error is %T %#v", err, err)
		ret.StatusError.Code = SSH_FX_FAILURE
		ret.StatusError.msg = err.Error()
		if err == io.EOF {
			ret.StatusError.Code = SSH_FX_EOF
		} else if errno, ok := err.(syscall.Errno); ok {
			ret.StatusError.Code = translateErrno(errno)
		} else if pathError, ok := err.(*os.PathError); ok {
			debug("StatusFromError: error is %T %#v", pathError.Err, pathError.Err)
			if errno, ok := pathError.Err.(syscall.Errno); ok {
				ret.StatusError.Code = translateErrno(errno)
			}
		}
	}
	return ret
}
