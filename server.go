package sftp

// sftp server counterpart

import (
	"encoding"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/pkg/errors"
)

const (
	sftpServerWorkerCount = 1
)

type ServerDriver interface {
	Stat(path string) (os.FileInfo, error)
	ListDir(path string) ([]os.FileInfo, error)
	DeleteDir(path string) error
	DeleteFile(path string) error
	Rename(oldPath string, newPath string) error
	MakeDir(path string) error
	GetFile(path string) (io.ReadCloser, error)
	PutFile(path string, reader io.Reader) error
	TranslatePath(root, homedir, path string) (string, error)
}

// Server is an SSH File Transfer Protocol (sftp) server.
// This is intended to provide the sftp subsystem to an ssh server daemon.
// This implementation currently supports most of sftp server protocol version 3,
// as specified at http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02
type Server struct {
	serverConn
	debugStream   io.Writer
	readOnly      bool
	pktChan       chan rxPacket
	openFiles     map[string]*fileHandle
	openFilesLock sync.RWMutex
	handleCount   int
	maxTxPacket   uint32
	driver        ServerDriver
}

type fileHandle struct {
	Path       string
	IsDir      bool
	Position   int
	TempHandle *os.File
}

func (svr *Server) nextHandle(f *fileHandle) string {
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
		if f.IsDir {
			return nil
		}

		defer func() {
			tmpName := f.TempHandle.Name()
			f.TempHandle.Close()
			os.Remove(tmpName)
		}()
		if _, err := f.TempHandle.Seek(0, 0); err != nil {
			return err
		}
		return svr.driver.PutFile(f.Path, f.TempHandle)
	}

	return syscall.EBADF
}

func (svr *Server) getHandle(handle string) (*fileHandle, bool) {
	svr.openFilesLock.RLock()
	defer svr.openFilesLock.RUnlock()
	f, ok := svr.openFiles[handle]
	return f, ok
}

type serverRespondablePacket interface {
	encoding.BinaryUnmarshaler
	id() uint32
	respond(svr *Server) error
}

// NewServer creates a new Server instance around the provided streams, serving
// content from the root of the filesystem.  Optionally, ServerOption
// functions may be specified to further configure the Server.
//
// A subsequent call to Serve() is required to begin serving files over SFTP.
func NewServer(rwc io.ReadWriteCloser, driver ServerDriver, options ...ServerOption) (*Server, error) {
	s := &Server{
		serverConn: serverConn{
			conn: conn{
				Reader:      rwc,
				WriteCloser: rwc,
			},
		},
		driver:      driver,
		debugStream: ioutil.Discard,
		pktChan:     make(chan rxPacket, sftpServerWorkerCount),
		openFiles:   make(map[string]*fileHandle),
		maxTxPacket: 1 << 15,
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
	pktType  fxp
	pktBytes []byte
}

// Up to N parallel servers
func (svr *Server) sftpServerWorker() error {
	for p := range svr.pktChan {
		var pkt interface {
			encoding.BinaryUnmarshaler
			id() uint32
		}
		var readonly = true
		switch p.pktType {
		case ssh_FXP_INIT:
			pkt = &sshFxInitPacket{}
		case ssh_FXP_LSTAT:
			pkt = &sshFxpLstatPacket{}
		case ssh_FXP_OPEN:
			pkt = &sshFxpOpenPacket{}
			// readonly handled specially below
		case ssh_FXP_CLOSE:
			pkt = &sshFxpClosePacket{}
		case ssh_FXP_READ:
			pkt = &sshFxpReadPacket{}
		case ssh_FXP_WRITE:
			pkt = &sshFxpWritePacket{}
			readonly = false
		case ssh_FXP_FSTAT:
			pkt = &sshFxpFstatPacket{}
		case ssh_FXP_SETSTAT:
			pkt = &sshFxpSetstatPacket{}
			readonly = false
		case ssh_FXP_FSETSTAT:
			pkt = &sshFxpFsetstatPacket{}
			readonly = false
		case ssh_FXP_OPENDIR:
			pkt = &sshFxpOpendirPacket{}
		case ssh_FXP_READDIR:
			pkt = &sshFxpReaddirPacket{}
		case ssh_FXP_REMOVE:
			pkt = &sshFxpRemovePacket{}
			readonly = false
		case ssh_FXP_MKDIR:
			pkt = &sshFxpMkdirPacket{}
			readonly = false
		case ssh_FXP_RMDIR:
			pkt = &sshFxpRmdirPacket{}
			readonly = false
		case ssh_FXP_REALPATH:
			pkt = &sshFxpRealpathPacket{}
		case ssh_FXP_STAT:
			pkt = &sshFxpStatPacket{}
		case ssh_FXP_RENAME:
			pkt = &sshFxpRenamePacket{}
			readonly = false
		case ssh_FXP_READLINK:
			pkt = &sshFxpReadlinkPacket{}
		case ssh_FXP_SYMLINK:
			pkt = &sshFxpSymlinkPacket{}
			readonly = false
		case ssh_FXP_EXTENDED:
			pkt = &sshFxpExtendedPacket{}
		default:
			return errors.Errorf("unhandled packet type: %s", p.pktType)
		}
		if err := pkt.UnmarshalBinary(p.pktBytes); err != nil {
			return err
		}

		// handle FXP_OPENDIR specially
		switch pkt := pkt.(type) {
		case *sshFxpOpenPacket:
			readonly = pkt.readonly()
		case *sshFxpExtendedPacket:
			readonly = pkt.SpecificPacket.readonly()
		}

		// If server is operating read-only and a write operation is requested,
		// return permission denied
		if !readonly && svr.readOnly {
			if err := svr.sendError(pkt, syscall.EPERM); err != nil {
				return errors.Wrap(err, "failed to send read only packet response")
			}
			continue
		}

		if err := handlePacket(svr, pkt); err != nil {
			return err
		}
	}
	return nil
}

func handlePacket(s *Server, p interface{}) error {
	switch p := p.(type) {
	case *sshFxInitPacket:
		return s.sendPacket(sshFxVersionPacket{sftpProtocolVersion, nil})
	case *sshFxpStatPacket:

		// stat the requested file
		info, err := s.driver.Stat(p.Path)
		if err != nil {
			return s.sendError(p, err)
		}
		return s.sendPacket(sshFxpStatResponse{
			ID:   p.ID,
			info: info,
		})
	case *sshFxpLstatPacket:
		// stat the requested file
		info, err := s.driver.Stat(p.Path)
		if err != nil {
			return s.sendError(p, err)
		}
		return s.sendPacket(sshFxpStatResponse{
			ID:   p.ID,
			info: info,
		})
	case *sshFxpFstatPacket:
		f, ok := s.getHandle(p.Handle)
		if !ok || f.IsDir {
			return s.sendError(p, syscall.EBADF)
		}

		info, err := s.driver.Stat(f.Path)
		if err != nil {
			return s.sendError(p, err)
		}

		return s.sendPacket(sshFxpStatResponse{
			ID:   p.ID,
			info: info,
		})
	case *sshFxpMkdirPacket:
		err := s.driver.MakeDir(p.Path)
		return s.sendError(p, err)
	case *sshFxpRmdirPacket:
		err := s.driver.DeleteDir(p.Path)
		return s.sendError(p, err)
	case *sshFxpRemovePacket:
		err := s.driver.DeleteFile(p.Filename)
		return s.sendError(p, err)
	case *sshFxpRenamePacket:
		err := s.driver.Rename(p.Oldpath, p.Newpath)
		return s.sendError(p, err)
	case *sshFxpSymlinkPacket:
		return s.sendError(p, fmt.Errorf("Not supported"))
	case *sshFxpClosePacket:
		return s.sendError(p, s.closeHandle(p.Handle))
	case *sshFxpReadlinkPacket:
		return s.sendError(p, fmt.Errorf("Not supported"))
	case *sshFxpRealpathPacket:
		f := s.driver.TranslatePath("", "", p.Path)
		return s.sendPacket(sshFxpNamePacket{
			ID: p.ID,
			NameAttrs: []sshFxpNameAttr{{
				Name:     f,
				LongName: f,
				Attrs:    emptyFileStat,
			}},
		})
	case *sshFxpOpendirPacket:
		handle := s.nextHandle(&fileHandle{
			Path:     p.Path,
			IsDir:    true,
			Position: 0,
		})
		return s.sendPacket(sshFxpHandlePacket{p.ID, handle})
	case *sshFxpReadPacket:
		f, ok := s.getHandle(p.Handle)
		if !ok || f.IsDir {
			return s.sendError(p, syscall.EBADF)
		}

		data := make([]byte, clamp(p.Len, s.maxTxPacket))
		n, err := f.TempHandle.ReadAt(data, int64(p.Offset))
		if err != nil && (err != io.EOF || n == 0) {
			return s.sendError(p, err)
		}
		return s.sendPacket(sshFxpDataPacket{
			ID:     p.ID,
			Length: uint32(n),
			Data:   data[:n],
		})
	case *sshFxpWritePacket:
		f, ok := s.getHandle(p.Handle)
		if !ok || f.IsDir {
			return s.sendError(p, syscall.EBADF)
		}

		_, err := f.TempHandle.WriteAt(p.Data, int64(p.Offset))
		return s.sendError(p, err)
	case serverRespondablePacket:
		err := p.respond(s)
		return errors.Wrap(err, "pkt.respond failed")
	default:
		return errors.Errorf("unexpected packet type %T", p)
	}
}

// Serve serves SFTP connections until the streams stop or the SFTP subsystem
// is stopped.
func (svr *Server) Serve() error {
	var wg sync.WaitGroup
	wg.Add(sftpServerWorkerCount)
	for i := 0; i < sftpServerWorkerCount; i++ {
		go func() {
			defer wg.Done()
			if err := svr.sftpServerWorker(); err != nil {
				svr.conn.Close() // shuts down recvPacket
			}
		}()
	}

	var err error
	var pktType uint8
	var pktBytes []byte
	for {
		pktType, pktBytes, err = svr.recvPacket()
		if err != nil {
			break
		}
		svr.pktChan <- rxPacket{fxp(pktType), pktBytes}
	}

	close(svr.pktChan) // shuts down sftpServerWorkers
	wg.Wait()          // wait for all workers to exit

	// close any still-open files
	for handle, file := range svr.openFiles {
		fmt.Fprintf(svr.debugStream, "sftp server file with handle %q left open: %v\n", handle, file.TempHandle.Name())
		file.TempHandle.Close()
	}
	return err // error from recvPacket
}

type id interface {
	id() uint32
}

// The init packet has no ID, so we just return a zero-value ID
func (p sshFxInitPacket) id() uint32 { return 0 }

type sshFxpStatResponse struct {
	ID   uint32
	info os.FileInfo
}

func (p sshFxpStatResponse) MarshalBinary() ([]byte, error) {
	b := []byte{ssh_FXP_ATTRS}
	b = marshalUint32(b, p.ID)
	b = marshalFileInfo(b, p.info)
	return b, nil
}

var emptyFileStat = []interface{}{uint32(0)}

func (p sshFxpOpenPacket) readonly() bool {
	return !p.hasPflags(ssh_FXF_WRITE)
}

func (p sshFxpOpenPacket) hasPflags(flags ...uint32) bool {
	for _, f := range flags {
		if p.Pflags&f == 0 {
			return false
		}
	}
	return true
}

func (p sshFxpOpenPacket) respond(svr *Server) error {
	if !p.hasPflags(ssh_FXF_READ) && !p.hasPflags(ssh_FXF_WRITE) {
		return svr.sendError(p, syscall.EINVAL)
	}

	tmpfile, err := ioutil.TempFile("", "sftp")
	if err != nil {
		return svr.sendError(p, err)
	}

	if p.hasPflags(ssh_FXF_CREAT) {
		if p.hasPflags(ssh_FXF_EXCL) {
			_, err := svr.driver.Stat(p.Path)
			if err == nil {
				return svr.sendError(p, syscall.EEXIST)
			}
		}
	}

	if !(p.hasPflags(ssh_FXF_CREAT) && p.hasPflags(ssh_FXF_TRUNC)) {
		fileReader, err := svr.driver.GetFile(p.Path)
		if err != nil {
			// TODO: Check if the error was actually 'file not found'
			if !p.hasPflags(ssh_FXF_CREAT) {
				return svr.sendError(p, err)
			}
		} else {
			reader := io.TeeReader(fileReader, tmpfile)
			ioutil.ReadAll(reader)
			fileReader.Close()
			tmpfile.Seek(0, 0)
		}
	}

	handle := svr.nextHandle(&fileHandle{
		Path:       p.Path,
		IsDir:      false,
		Position:   0,
		TempHandle: tmpfile,
	})
	return svr.sendPacket(sshFxpHandlePacket{p.ID, handle})
}

func generateLongName(fileInfo os.FileInfo) string {
	// The format of the `longname' field is unspecified by this protocol.
	// It MUST be suitable for use in the output of a directory listing
	// command (in fact, the recommended operation for a directory listing
	// command is to simply display this data).  However, clients SHOULD NOT
	// attempt to parse the longname field for file attributes; they SHOULD
	// use the attrs field instead.
	//  - SFTP Specification, https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02

	// Despite this, many SFTP clients DO parse the long name and expect OpenSSH's format. This function produces a
	// longname that matches that format.
	dateFormat := "Jan _2 15:04"
	if fileInfo.ModTime().Year() != time.Now().Year() {
		dateFormat = "Jan _2  2006"
	}
	permissionFlags := "-rw-------"
	if fileInfo.IsDir() {
		permissionFlags = "drw-------"
	}
	return fmt.Sprintf("%-10s %3d %-8s %-8s %8d %-12s %-s", permissionFlags,
		1, "owner", "owner", fileInfo.Size(), fileInfo.ModTime().Format(dateFormat), fileInfo.Name())
}

func (p sshFxpReaddirPacket) respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok || !f.IsDir {
		return svr.sendError(p, syscall.EBADF)
	}

	// If we've returned any files, we must have returned all of them, because the driver does not yet provide a way to
	// page through results. So simply return EOF.
	if f.Position > 0 {
		return svr.sendError(p, io.EOF)
	}

	files, err := svr.driver.ListDir(f.Path)
	if err != nil {
		return svr.sendError(p, err)
	}

	// For compatibility, include a '.' and '..' directory.
	files = append(files, &fileInfo{
		name:  ".",
		size:  4096,
		mtime: time.Unix(1, 0),
		mode:  os.ModeDir,
	})
	files = append(files, &fileInfo{
		name:  "..",
		size:  4096,
		mtime: time.Unix(1, 0),
		mode:  os.ModeDir,
	})

	ret := sshFxpNamePacket{ID: p.ID}
	for _, dirent := range files {

		ret.NameAttrs = append(ret.NameAttrs, sshFxpNameAttr{
			Name:     dirent.Name(),
			LongName: generateLongName(dirent),
			Attrs:    []interface{}{dirent},
		})
	}
	f.Position = len(files)
	return svr.sendPacket(ret)
}

func (p sshFxpSetstatPacket) respond(svr *Server) error {
	debug("setstat name \"%s\"", p.Path)
	if (p.Flags & ssh_FILEXFER_ATTR_SIZE) != 0 {
		return svr.sendError(p, syscall.ENOSYS)
	}
	// Silently ignore other actions

	return svr.sendError(p, nil)
}

func (p sshFxpFsetstatPacket) respond(svr *Server) error {
	f, ok := svr.getHandle(p.Handle)
	if !ok || f.IsDir {
		return svr.sendError(p, syscall.EBADF)
	}

	debug("fsetstat name \"%s\"", f.Path)
	if (p.Flags & ssh_FILEXFER_ATTR_SIZE) != 0 {
		return svr.sendError(p, syscall.ENOSYS)
	}
	// Silently ignore other actions

	return svr.sendError(p, nil)
}

// translateErrno translates a syscall error number to a SFTP error code.
func translateErrno(errno syscall.Errno) uint32 {
	switch errno {
	case 0:
		return ssh_FX_OK
	case syscall.ENOENT:
		return ssh_FX_NO_SUCH_FILE
	case syscall.EPERM:
		return ssh_FX_PERMISSION_DENIED
	case syscall.ENOSYS:
		return ssh_FX_OP_UNSUPPORTED
	}

	return ssh_FX_FAILURE
}

func statusFromError(p id, err error) sshFxpStatusPacket {
	ret := sshFxpStatusPacket{
		ID: p.id(),
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
			ret.StatusError.Code = translateErrno(errno)
		} else if pathError, ok := err.(*os.PathError); ok {
			debug("statusFromError: error is %T %#v", pathError.Err, pathError.Err)
			if errno, ok := pathError.Err.(syscall.Errno); ok {
				ret.StatusError.Code = translateErrno(errno)
			}
		}
	}
	return ret
}

func clamp(v, max uint32) uint32 {
	if v > max {
		return max
	}
	return v
}
