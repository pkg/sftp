package sftp

import (
	"syscall"
)

func (p sshFxpExtendedPacketStatVFS) respond(svr *Server) responsePacket {
	return statusFromError(p, syscall.EPLAN9)
}
