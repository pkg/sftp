package openssh

import (
	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

const extensionPosixRename = "posix-rename@openssh.com"

// RegisterExtensionPosixRename registers the "posix-rename@openssh.com" extended packet with the encoding/ssh/filexfer package.
func RegisterExtensionPosixRename() {
	sshfx.RegisterExtendedPacketType(extensionPosixRename, func() sshfx.ExtendedData {
		return new(PosixRenameExtendedPacket)
	})
}

// ExtensionPosixRename returns an ExtensionPair suitable to append into an sshfx.InitPacket or sshfx.VersionPacket.
func ExtensionPosixRename() *sshfx.ExtensionPair {
	return &sshfx.ExtensionPair{
		Name: extensionPosixRename,
		Data: "1",
	}
}

// PosixRenameExtendedPacket defines the posix-rename@openssh.com extend packet.
type PosixRenameExtendedPacket struct {
	OldPath string
	NewPath string
}

// Type returns the SSH_FXP_EXTENDED packet type.
func (ep *PosixRenameExtendedPacket) Type() sshfx.PacketType {
	return sshfx.PacketTypeExtended
}

// MarshalPacket returns ep as a two-part binary encoding of the full extended packet.
func (ep *PosixRenameExtendedPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	p := &sshfx.ExtendedPacket{
		ExtendedRequest: extensionPosixRename,

		Data: ep,
	}
	return p.MarshalPacket(reqid, b)
}

// MarshalInto encodes ep into the binary encoding of the hardlink@openssh.com extended packet-specific data.
func (ep *PosixRenameExtendedPacket) MarshalInto(buf *sshfx.Buffer) {
	buf.AppendString(ep.OldPath)
	buf.AppendString(ep.NewPath)
}

// MarshalBinary encodes ep into the binary encoding of the hardlink@openssh.com extended packet-specific data.
//
// NOTE: This _only_ encodes the packet-specific data, it does not encode the full extended packet.
func (ep *PosixRenameExtendedPacket) MarshalBinary() ([]byte, error) {
	// string(oldpath) + string(newpath)
	size := 4 + len(ep.OldPath) + 4 + len(ep.NewPath)

	buf := sshfx.NewBuffer(make([]byte, 0, size))
	ep.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom decodes the hardlink@openssh.com extended packet-specific data from buf.
func (ep *PosixRenameExtendedPacket) UnmarshalFrom(buf *sshfx.Buffer) (err error) {
	if ep.OldPath, err = buf.ConsumeString(); err != nil {
		return err
	}

	if ep.NewPath, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

// UnmarshalBinary decodes the hardlink@openssh.com extended packet-specific data into ep.
func (ep *PosixRenameExtendedPacket) UnmarshalBinary(data []byte) (err error) {
	return ep.UnmarshalFrom(sshfx.NewBuffer(data))
}
