// Package sftp implements the SSH File Transfer Protocol as described in
// https://filezilla-project.org/specs/draft-ietf-secsh-filexfer-02.txt
package sftp

import (
	"fmt"
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

const (
	SSH_FX_OK                = 0
	SSH_FX_EOF               = 1
	SSH_FX_NO_SUCH_FILE      = 2
	SSH_FX_PERMISSION_DENIED = 3
	SSH_FX_FAILURE           = 4
	SSH_FX_BAD_MESSAGE       = 5
	SSH_FX_NO_CONNECTION     = 6
	SSH_FX_CONNECTION_LOST   = 7
	SSH_FX_OP_UNSUPPORTED    = 8

	// see draft-ietf-secsh-filexfer-13
	// https://tools.ietf.org/html/draft-ietf-secsh-filexfer-13#section-9.1
	SSH_FX_INVALID_HANDLE              = 9
	SSH_FX_NO_SUCH_PATH                = 10
	SSH_FX_FILE_ALREADY_EXISTS         = 11
	SSH_FX_WRITE_PROTECT               = 12
	SSH_FX_NO_MEDIA                    = 13
	SSH_FX_NO_SPACE_ON_FILESYSTEM      = 14
	SSH_FX_QUOTA_EXCEEDED              = 15
	SSH_FX_UNKNOWN_PRINCIPAL           = 16
	SSH_FX_LOCK_CONFLICT               = 17
	SSH_FX_DIR_NOT_EMPTY               = 18
	SSH_FX_NOT_A_DIRECTORY             = 19
	SSH_FX_INVALID_FILENAME            = 20
	SSH_FX_LINK_LOOP                   = 21
	SSH_FX_CANNOT_DELETE               = 22
	SSH_FX_INVALID_PARAMETER           = 23
	SSH_FX_FILE_IS_A_DIRECTORY         = 24
	SSH_FX_BYTE_RANGE_LOCK_CONFLICT    = 25
	SSH_FX_BYTE_RANGE_LOCK_REFUSED     = 26
	SSH_FX_DELETE_PENDING              = 27
	SSH_FX_FILE_CORRUPT                = 28
	SSH_FX_OWNER_INVALID               = 29
	SSH_FX_GROUP_INVALID               = 30
	SSH_FX_NO_MATCHING_BYTE_RANGE_LOCK = 31
)

const (
	SSH_FXF_READ   = 0x00000001
	SSH_FXF_WRITE  = 0x00000002
	SSH_FXF_APPEND = 0x00000004
	SSH_FXF_CREAT  = 0x00000008
	SSH_FXF_TRUNC  = 0x00000010
	SSH_FXF_EXCL   = 0x00000020
)

type Fxpkt uint8

func (f Fxpkt) String() string {
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

type unexpectedPacketErr struct {
	want, got uint8
}

func (u *unexpectedPacketErr) Error() string {
	return fmt.Sprintf("sftp: unexpected packet: want %v, got %v", Fxpkt(u.want), Fxpkt(u.got))
}

func unimplementedPacketErr(u uint8) error {
	return fmt.Errorf("sftp: unimplemented packet type: got %v", Fxpkt(u))
}

type unexpectedIDErr struct{ want, got uint32 }

func (u *unexpectedIDErr) Error() string {
	return fmt.Sprintf("sftp: unexpected id: want %v, got %v", u.want, u.got)
}

func unimplementedSeekWhence(whence int) error {
	return fmt.Errorf("sftp: unimplemented seek whence %v", whence)
}

func unexpectedCount(want, got uint32) error {
	return fmt.Errorf("sftp: unexpected count: want %v, got %v", want, got)
}

type unexpectedVersionErr struct{ want, got uint32 }

func (u *unexpectedVersionErr) Error() string {
	return fmt.Sprintf("sftp: unexpected server version: want %v, got %v", u.want, u.got)
}

// A StatusError is returned when an SFTP operation fails, and provides
// additional information about the failure.
type StatusError struct {
	Code      uint32
	msg, lang string
}

func (s *StatusError) Error() string { return fmt.Sprintf("sftp: %q (%v)", s.msg, fx(s.Code)) }
