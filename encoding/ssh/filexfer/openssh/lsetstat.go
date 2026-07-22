package openssh

import (
	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

const extensionLSetStat = "lsetstat@openssh.com"

// ExtensionLSetStat returns an ExtensionPair suitable to append into an sshfx.InitPacket or sshfx.VersionPacket.
func ExtensionLSetStat() *sshfx.ExtensionPair {
	return &sshfx.ExtensionPair{
		Name: extensionLSetStat,
		Data: "1",
	}
}

// LSetStatExtendedPacket defines the fsync@openssh.com extend packet.
type LSetStatExtendedPacket struct {
	Path  string
	Attrs sshfx.Attributes
}

// Type returns the SSH_FXP_EXTENDED packet type.
func (ep *LSetStatExtendedPacket) Type() sshfx.PacketType {
	return sshfx.PacketTypeExtended
}

// MarshalSize returns the number of bytes that the extended request data would marshal into.
func (ep *LSetStatExtendedPacket) MarshalSize() int {
	// string(handle)
	return 4 + len(ep.Path) + ep.Attrs.MarshalSize()
}

// ExtendedRequest returns the SSH_FXP_EXTENDED extended-request field associated with this packet type.
func (ep *LSetStatExtendedPacket) ExtendedRequest() string {
	return extensionLSetStat
}

// GetPath returns the path field of the packet.
func (ep *LSetStatExtendedPacket) GetPath() string {
	return ep.Path
}

// GetAttrs returns the attrs field of the packet.
func (ep *LSetStatExtendedPacket) GetAttrs() *sshfx.Attributes {
	return &ep.Attrs
}

// MarshalPacket returns ep as a two-part binary encoding of the full extended packet.
func (ep *LSetStatExtendedPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	p := &sshfx.ExtendedPacket{
		ExtendedRequest: extensionLSetStat,

		Data: ep,
	}
	return p.MarshalPacket(reqid, b)
}

// MarshalInto encodes ep into the binary encoding of the fsync@openssh.com extended packet-specific data.
func (ep *LSetStatExtendedPacket) MarshalInto(buf *sshfx.Buffer) {
	buf.AppendString(ep.Path)
	ep.Attrs.MarshalInto(buf)
}

// MarshalBinary encodes ep into the binary encoding of the fsync@openssh.com extended packet-specific data.
//
// NOTE: This _only_ encodes the packet-specific data, it does not encode the full extended packet.
func (ep *LSetStatExtendedPacket) MarshalBinary() ([]byte, error) {
	buf := sshfx.NewMarshalBuffer(ep.MarshalSize())
	ep.MarshalInto(buf)
	return buf.Bytes(), nil
}

// UnmarshalFrom decodes the fsync@openssh.com extended packet-specific data from buf.
func (ep *LSetStatExtendedPacket) UnmarshalFrom(buf *sshfx.Buffer) (err error) {
	*ep = LSetStatExtendedPacket{
		Path: buf.ConsumeString(),
	}
	ep.Attrs.UnmarshalFrom(buf)

	return buf.Err
}

// UnmarshalBinary decodes the fsync@openssh.com extended packet-specific data into ep.
func (ep *LSetStatExtendedPacket) UnmarshalBinary(data []byte) (err error) {
	return ep.UnmarshalFrom(sshfx.NewBuffer(data))
}
