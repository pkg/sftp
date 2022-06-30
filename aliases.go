package sftp

type RequestPacket = requestPacket

type OpenPacket = sshFxpOpenPacket
type ClosePacket = sshFxpClosePacket
type ReadPacket = sshFxpReadPacket
type WritePacket = sshFxpWritePacket
type LstatPacket = sshFxpLstatPacket
type FstatPacket = sshFxpFstatPacket
type SetstatPacket = sshFxpSetstatPacket
type FsetstatPacket = sshFxpFsetstatPacket
type OpendirPacket = sshFxpOpendirPacket
type ReaddirPacket = sshFxpReaddirPacket
type RemovePacket = sshFxpRemovePacket
type MkdirPacket = sshFxpMkdirPacket
type RmdirPacket = sshFxpRmdirPacket
type RealpathPacket = sshFxpRealpathPacket
type StatPacket = sshFxpStatPacket
type RenamePacket = sshFxpRenamePacket
type ReadlinkPacket = sshFxpReadlinkPacket
type SymlinkPacket = sshFxpSymlinkPacket
