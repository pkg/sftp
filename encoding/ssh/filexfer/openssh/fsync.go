package openssh

import (
	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

const extensionFSync = "fsync@openssh.com"

// ExtensionFSync returns an ExtensionPair suitable to append into an sshfx.InitPacket or sshfx.VersionPacket.
func ExtensionFSync() *sshfx.ExtensionPair {
	return &sshfx.ExtensionPair{
		Name: extensionFSync,
		Data: "1",
	}
}

// FSyncExtendedPacket defines the fsync@openssh.com extend packet.
type FSyncExtendedPacket struct {
	Handle string
}

// Type returns the SSH_FXP_EXTENDED packet type.
func (ep *FSyncExtendedPacket) Type() sshfx.PacketType {
	return sshfx.PacketTypeExtended
}

// ExtendedRequest returns the SSH_FXP_EXTENDED extended-request field associated with this packet type.
func (ep *FSyncExtendedPacket) ExtendedRequest() string {
	return extensionFSync
}

// GetHandle returns the handle field of the packet.
func (ep *FSyncExtendedPacket) GetHandle() string {
	return ep.Handle
}

// MarshalPacket returns ep as a two-part binary encoding of the full extended packet.
func (ep *FSyncExtendedPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	p := &sshfx.ExtendedPacket{
		ExtendedRequest: extensionFSync,

		Data: ep,
	}
	return p.MarshalPacket(reqid, b)
}

// MarshalInto encodes ep into the binary encoding of the fsync@openssh.com extended packet-specific data.
func (ep *FSyncExtendedPacket) MarshalInto(buf *sshfx.Buffer) {
	buf.AppendString(ep.Handle)
}

// MarshalBinary encodes ep into the binary encoding of the fsync@openssh.com extended packet-specific data.
//
// NOTE: This _only_ encodes the packet-specific data, it does not encode the full extended packet.
func (ep *FSyncExtendedPacket) MarshalBinary() ([]byte, error) {
	// string(handle)
	size := 4 + len(ep.Handle)

	buf := sshfx.NewBuffer(make([]byte, 0, size))
	ep.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom decodes the fsync@openssh.com extended packet-specific data from buf.
func (ep *FSyncExtendedPacket) UnmarshalFrom(buf *sshfx.Buffer) (err error) {
	*ep = FSyncExtendedPacket{
		Handle: buf.ConsumeString(),
	}

	return buf.Err
}

// UnmarshalBinary decodes the fsync@openssh.com extended packet-specific data into ep.
func (ep *FSyncExtendedPacket) UnmarshalBinary(data []byte) (err error) {
	return ep.UnmarshalFrom(sshfx.NewBuffer(data))
}
