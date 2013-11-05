// Package sftp implements the SSH File Transfer Protocol as described in
// https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-02.txt
package sftp

import (
	"fmt"
	"io"

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
)

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
	return sftp, sftp.sendInit()
}

type ClientConn struct {
	w io.WriteCloser
	r io.Reader
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

type unexpectedPacketErr struct {
	want, got uint8
}

func (u *unexpectedPacketErr) Error() string {
	return fmt.Sprintf("sftp: unexpected packet: want %v, got %v", u.want, u.got)
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
