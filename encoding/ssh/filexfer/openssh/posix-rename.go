package openssh

import (
	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

const extensionPOSIXRename = "posix-rename@openssh.com"

// ExtensionPOSIXRename returns an ExtensionPair suitable to append into an sshfx.InitPacket or sshfx.VersionPacket.
func ExtensionPOSIXRename() *sshfx.ExtensionPair {
	return &sshfx.ExtensionPair{
		Name: extensionPOSIXRename,
		Data: "1",
	}
}

// POSIXRenameExtendedPacket defines the posix-rename@openssh.com extend packet.
type POSIXRenameExtendedPacket struct {
	OldPath string
	NewPath string
}

// Type returns the SSH_FXP_EXTENDED packet type.
func (ep *POSIXRenameExtendedPacket) Type() sshfx.PacketType {
	return sshfx.PacketTypeExtended
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (ep *POSIXRenameExtendedPacket) MarshalSize() int {
	// string(oldpath) + string(newpath)
	return 4 + len(ep.OldPath) + 4 + len(ep.NewPath)
}

// ExtendedRequest returns the SSH_FXP_EXTENDED extended-request field associated with this packet type.
func (ep *POSIXRenameExtendedPacket) ExtendedRequest() string {
	return extensionPOSIXRename
}

// MarshalPacket returns ep as a two-part binary encoding of the full extended packet.
func (ep *POSIXRenameExtendedPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	p := &sshfx.ExtendedPacket{
		ExtendedRequest: extensionPOSIXRename,

		Data: ep,
	}
	return p.MarshalPacket(reqid, b)
}

// MarshalInto encodes ep into the binary encoding of the hardlink@openssh.com extended packet-specific data.
func (ep *POSIXRenameExtendedPacket) MarshalInto(buf *sshfx.Buffer) {
	buf.AppendString(ep.OldPath)
	buf.AppendString(ep.NewPath)
}

// MarshalBinary encodes ep into the binary encoding of the hardlink@openssh.com extended packet-specific data.
//
// NOTE: This _only_ encodes the packet-specific data, it does not encode the full extended packet.
func (ep *POSIXRenameExtendedPacket) MarshalBinary() ([]byte, error) {
	buf := sshfx.NewBuffer(make([]byte, 0, ep.MarshalSize()))
	ep.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom decodes the hardlink@openssh.com extended packet-specific data from buf.
func (ep *POSIXRenameExtendedPacket) UnmarshalFrom(buf *sshfx.Buffer) (err error) {
	*ep = POSIXRenameExtendedPacket{
		OldPath: buf.ConsumeString(),
		NewPath: buf.ConsumeString(),
	}

	return buf.Err
}

// UnmarshalBinary decodes the hardlink@openssh.com extended packet-specific data into ep.
func (ep *POSIXRenameExtendedPacket) UnmarshalBinary(data []byte) (err error) {
	return ep.UnmarshalFrom(sshfx.NewBuffer(data))
}
