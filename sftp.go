// Package sftp implements the SSH File Transfer Protocol as described in
// https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-02.txt
package sftp

import (
	"fmt"
	"io"
	"sync"

	"code.google.com/p/go.crypto/ssh"
)

const (
	SSH_FXP_INIT           = 1
	SSH_FXP_VERSION        = 2
	SSH_FXP_OPEN           = 3
	SSH_FXP_CLOSE          = 4
	SSH_FXP_READ           = 5
	SSH_FXP_WRITE          = 6
	SSH_FXP_LSTAT          = 7
	SSH_FXP_FSTAT          = 8
	SSH_FXP_SETSTAT        = 9
	SSH_FXP_FSETSTAT       = 10
	SSH_FXP_OPENDIR        = 11
	SSH_FXP_READDIR        = 12
	SSH_FXP_REMOVE         = 13
	SSH_FXP_MKDIR          = 14
	SSH_FXP_RMDIR          = 15
	SSH_FXP_REALPATH       = 16
	SSH_FXP_STAT           = 17
	SSH_FXP_RENAME         = 18
	SSH_FXP_READLINK       = 19
	SSH_FXP_SYMLINK        = 20
	SSH_FXP_STATUS         = 101
	SSH_FXP_HANDLE         = 102
	SSH_FXP_DATA           = 103
	SSH_FXP_NAME           = 104
	SSH_FXP_ATTRS          = 105
	SSH_FXP_EXTENDED       = 200
	SSH_FXP_EXTENDED_REPLY = 201

	SSH_FX_OK                = 0
	SSH_FX_EOF               = 1
	SSH_FX_NO_SUCH_FILE      = 2
	SSH_FX_PERMISSION_DENIED = 3
	SSH_FX_FAILURE           = 4
	SSH_FX_BAD_MESSAGE       = 5
	SSH_FX_NO_CONNECTION     = 6
	SSH_FX_CONNECTION_LOST   = 7
	SSH_FX_OP_UNSUPPORTED    = 8
)

type fxp uint8

func (f fxp) String() string {
	switch f {
	case SSH_FXP_INIT:
		return "SSH_FXP_INIT"
	case SSH_FXP_VERSION:
		return "SSH_FXP_VERSION"
	case SSH_FXP_OPEN:
		return "SSH_FXP_OPEN"
	case SSH_FXP_CLOSE:
		return "SSH_FXP_CLOSE"
	case SSH_FXP_READ:
		return "SSH_FXP_READ"
	case SSH_FXP_WRITE:
		return "SSH_FXP_WRITE"
	case SSH_FXP_LSTAT:
		return "SSH_FXP_LSTAT"
	case SSH_FXP_FSTAT:
		return "SSH_FXP_FSTAT"
	case SSH_FXP_SETSTAT:
		return "SSH_FXP_SETSTAT"
	case SSH_FXP_FSETSTAT:
		return "SSH_FXP_FSETSTAT"
	case SSH_FXP_OPENDIR:
		return "SSH_FXP_OPENDIR"
	case SSH_FXP_READDIR:
		return "SSH_FXP_READDIR"
	case SSH_FXP_REMOVE:
		return "SSH_FXP_REMOVE"
	case SSH_FXP_MKDIR:
		return "SSH_FXP_MKDIR"
	case SSH_FXP_RMDIR:
		return "SSH_FXP_RMDIR"
	case SSH_FXP_REALPATH:
		return "SSH_FXP_REALPATH"
	case SSH_FXP_STAT:
		return "SSH_FXP_STAT"
	case SSH_FXP_RENAME:
		return "SSH_FXP_RENAME"
	case SSH_FXP_READLINK:
		return "SSH_FXP_READLINK"
	case SSH_FXP_SYMLINK:
		return "SSH_FXP_SYMLINK"
	case SSH_FXP_STATUS:
		return "SSH_FXP_STATUS"
	case SSH_FXP_HANDLE:
		return "SSH_FXP_HANDLE"
	case SSH_FXP_DATA:
		return "SSH_FXP_DATA"
	case SSH_FXP_NAME:
		return "SSH_FXP_NAME"
	case SSH_FXP_ATTRS:
		return "SSH_FXP_ATTRS"
	case SSH_FXP_EXTENDED:
		return "SSH_FXP_EXTENDED"
	case SSH_FXP_EXTENDED_REPLY:
		return "SSH_FXP_EXTENDED_REPLY"
	default:
		return "unknown"
	}
}

type fx uint8

func (f fx) String() string {
	switch f {
	case SSH_FX_OK:
		return "SSH_FX_OK"
	case SSH_FX_EOF:
		return "SSH_FX_EOF"
	case SSH_FX_NO_SUCH_FILE:
		return "SSH_FX_NO_SUCH_FILE"
	case SSH_FX_PERMISSION_DENIED:
		return "SSH_FX_PERMISSION_DENIED"
	case SSH_FX_FAILURE:
		return "SSH_FX_FAILURE"
	case SSH_FX_BAD_MESSAGE:
		return "SSH_FX_BAD_MESSAGE"
	case SSH_FX_NO_CONNECTION:
		return "SSH_FX_NO_CONNECTION"
	case SSH_FX_CONNECTION_LOST:
		return "SSH_FX_CONNECTION_LOST"
	case SSH_FX_OP_UNSUPPORTED:
		return "SSH_FX_OP_UNSUPPORTED"
	default:
		return "unknown"
	}
}

type packet struct {
	length uint32
	typ    uint8
	data   []byte
}

// New creates a new sftp client on conn.
func NewClient(conn *ssh.ClientConn) (*ClientConn, error) {
	s, err := conn.NewSession()
	if err != nil {
		return nil, err
	}
	if err := s.RequestSubsystem("sftp"); err != nil {
		return nil, err
	}
	pw, err := s.StdinPipe()
	if err != nil {
		return nil, err
	}
	pr, err := s.StdoutPipe()
	if err != nil {
		return nil, err
	}
	sftp := &ClientConn{
		w: pw,
		r: pr,
	}
	if err := sftp.sendInit(); err != nil {
		return nil, err
	}
	return sftp, sftp.recvVersion()
}

type ClientConn struct {
	w      io.WriteCloser
	r      io.Reader
	mu     sync.Mutex
	nextid uint32
}

func (c *ClientConn) Close() error { return c.w.Close() }

func (c *ClientConn) sendInit() error {
	type packet struct {
		Type       byte
		Version    uint32
		Extensions []struct {
			Name, Data string
		}
	}
	return sendPacket(c.w, packet{
		Type:    SSH_FXP_INIT,
		Version: 3, // http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02
	})
}

// returns the current value of c.nextid and increments it
// callers is expected to hold c.mu
func (c *ClientConn) nextId() uint32 {
	v := c.nextid
	c.nextid++
	return v
}

type unexpectedPacketErr struct {
	want, got uint8
}

func (u *unexpectedPacketErr) Error() string {
	return fmt.Sprintf("sftp: unexpected packet: want %v, got %v", fxp(u.want), fxp(u.got))
}

type unimplementedPacketErr uint8

func (u unimplementedPacketErr) Error() string {
	return fmt.Sprintf("sftp: unimplemented packet type: got %v", fxp(u))
}

type unexpectedIdErr struct{ want, got uint32 }

func (u *unexpectedIdErr) Error() string {
	return fmt.Sprintf("sftp: unexpected id: want %v, got %v", u.want, u.got)
}

func (c *ClientConn) recvVersion() error {
	typ, _, err := recvPacket(c.r)
	if err != nil {
		return err
	}
	if typ != SSH_FXP_VERSION {
		return &unexpectedPacketErr{SSH_FXP_VERSION, typ}
	}
	return nil
}

type Walker struct {
	c      *ClientConn
	handle string
}

type StatusError struct {
	Code      uint32
	msg, lang string
}

func (s *StatusError) Error() string { return fmt.Sprintf("sftp: %q (%v)", s.msg, fx(s.Code)) }

// Walk returns a new Walker rooted at root.
func (c *ClientConn) Walk(root string) (*Walker, error) {
	type packet struct {
		Type byte
		Id   uint32
		Path string
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type: SSH_FXP_OPENDIR,
		Id:   id,
		Path: root,
	}); err != nil {
		return nil, err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return nil, err
	}
	switch typ {
	case SSH_FXP_HANDLE:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		handle, _ := unmarshalString(data)
		return &Walker{
			c:      c,
			handle: handle,
		}, nil
	case SSH_FXP_STATUS:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		code, data := unmarshalUint32(data)
		msg, data := unmarshalString(data)
		lang, _ := unmarshalString(data)
		return nil, &StatusError{
			Code: code,
			msg:  msg,
			lang: lang,
		}
	default:
		return nil, unimplementedPacketErr(typ)
	}
}
