package sftp

import (
	"encoding"
	"github.com/pkg/errors"
)

type SSHFxpExtendedPacket struct {
	ID              uint32
	ExtendedRequest string
	SpecificPacket  interface {
		encoding.BinaryUnmarshaler
		Id() uint32
		Readonly() bool
	}
}

func (p SSHFxpExtendedPacket) Id() uint32 { return p.ID }
func (p SSHFxpExtendedPacket) Readonly() bool {
	if p.SpecificPacket == nil {
		return true
	}
	return p.SpecificPacket.Readonly()
}

func (p *SSHFxpExtendedPacket) UnmarshalBinary(b []byte) error {
	var err error
	bOrig := b
	if p.ID, b, err = unmarshalUint32Safe(b); err != nil {
		return err
	} else if p.ExtendedRequest, _, err = unmarshalStringSafe(b); err != nil {
		return err
	}

	// specific unmarshalling
	switch p.ExtendedRequest {
	case "statvfs@openssh.com":
		p.SpecificPacket = &SSHFxpExtendedPacketStatVFS{}
	case "posix-rename@openssh.com":
		p.SpecificPacket = &SSHFxpExtendedPacketPosixRename{}
	default:
		return errors.Wrapf(errUnknownExtendedPacket, "packet type %v", p.SpecificPacket)
	}

	return p.SpecificPacket.UnmarshalBinary(bOrig)
}
