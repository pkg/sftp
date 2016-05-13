package sftp

import "fmt"

func FilesystemResponder(pktType Fxpkt) (ServerRespondablePacket, error) {
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
	return pkt, nil
}
