package sftp

// Type the packets using interfaces in a couple ways
type hasPath interface {
	getPath() string
}

type hasHandle interface {
	getHandle() string
}

//// define types by adding methods
// hasPath
func (p sshFxpOpendirPacket) getPath() string  { return p.Path }
func (p sshFxpLstatPacket) getPath() string    { return p.Path }
func (p sshFxpStatPacket) getPath() string     { return p.Path }
func (p sshFxpRmdirPacket) getPath() string    { return p.Path }
func (p sshFxpReadlinkPacket) getPath() string { return p.Path }
func (p sshFxpRealpathPacket) getPath() string { return p.Path }
func (p sshFxpOpenPacket) getPath() string     { return p.Path }
func (p sshFxpMkdirPacket) getPath() string    { return p.Path }
func (p sshFxpSetstatPacket) getPath() string  { return p.Path }
func (p sshFxpStatvfsPacket) getPath() string  { return p.Path }
func (p sshFxpRemovePacket) getPath() string   { return p.Filename }
func (p sshFxpRenamePacket) getPath() string   { return p.Oldpath }
func (p sshFxpSymlinkPacket) getPath() string  { return p.Targetpath }

// hasHandle
func (p sshFxpReaddirPacket) getHandle() string  { return p.Handle }
func (p sshFxpFstatPacket) getHandle() string    { return p.Handle }
func (p sshFxpClosePacket) getHandle() string    { return p.Handle }
func (p sshFxpReadPacket) getHandle() string     { return p.Handle }
func (p sshFxpWritePacket) getHandle() string    { return p.Handle }
func (p sshFxpFsetstatPacket) getHandle() string { return p.Handle }
