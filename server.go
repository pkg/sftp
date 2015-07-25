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
}

type nativeFs struct {
}

func (nfs *nativeFs) Lstat(p string) (os.FileInfo, error) { return os.Lstat(p) }

type Server struct {
	in            io.Reader
	out           io.Writer
	rootDir       string
	lastId        uint32
	fs            FileSystem
	pktChan       chan serverRespondablePacket
	openFiles     map[string]*svrFile
	openFilesLock *sync.Mutex
}

type serverRespondablePacket interface {
	encoding.BinaryUnmarshaler
	respond(svr *Server) error
}

type svrFile struct {
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
		openFiles:     map[string]*svrFile{},
		openFilesLock: &sync.Mutex{},
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
	case ssh_FXP_CLOSE:
	case ssh_FXP_READ:
	case ssh_FXP_WRITE:
	case ssh_FXP_FSTAT:
	case ssh_FXP_SETSTAT:
	case ssh_FXP_FSETSTAT:
	case ssh_FXP_OPENDIR:
	case ssh_FXP_READDIR:
	case ssh_FXP_REMOVE:
	case ssh_FXP_MKDIR:
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

type sshFxpStatusPacket struct {
	Id uint32
	StatusError
}

func (p sshFxpStatusPacket) MarshalBinary() ([]byte, error) {
	b := []byte{ssh_FXP_STATUS}
	b = marshalUint32(b, p.Id)
	b = marshalStatus(b, p.StatusError)
	return b, nil
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
			Code: ssh_FX_FAILURE,
			msg:  err.Error(),
			lang: "",
		},
	}
	debug("statusFromError: error is %T %#v", err, err)
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
	return ret
}
