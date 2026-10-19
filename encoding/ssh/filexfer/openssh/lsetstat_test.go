package openssh

import (
	"bytes"
	"testing"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

var _ sshfx.PacketMarshaller = &LSetStatExtendedPacket{}

func init() {
	sshfx.RegisterExtendedPacketType[LSetStatExtendedPacket]()
}

func TestLSetStatExtendedPacket(t *testing.T) {
	const (
		id                  = 42
		path                = "/foo"
		perm sshfx.FileMode = 0x87654321
	)

	ep := &LSetStatExtendedPacket{
		Path: path,
		Attrs: sshfx.Attributes{
			Flags:       sshfx.AttrPermissions,
			Permissions: perm,
		},
	}

	data, err := sshfx.ComposePacket(ep.MarshalPacket(id, nil))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 45,
		200,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 20, 'l', 's', 'e', 't', 's', 't', 'a', 't', '@', 'o', 'p', 'e', 'n', 's', 's', 'h', '.', 'c', 'o', 'm',
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
		0x00, 0x00, 0x00, 0x04,
		0x87, 0x65, 0x43, 0x21,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("MarshalPacket() = %X, but wanted %X", data, want)
	}

	var p sshfx.ExtendedPacket

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(sshfx.NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.ExtendedRequest != extensionLSetStat {
		t.Errorf("UnmarshalPacketBody(): ExtendedRequest was %q, but expected %q", p.ExtendedRequest, extensionLSetStat)
	}

	ep, ok := p.Data.(*LSetStatExtendedPacket)
	if !ok {
		t.Fatalf("UnmarshaledPacketBody(): Data was type %T, but expected *LSetStatExtendedPacket", p.Data)
	}

	if ep.Path != path {
		t.Errorf("UnmarshalPacketBody(): Path was %q, but expected %q", ep.Path, path)
	}

	if ep.Attrs.Flags != sshfx.AttrPermissions {
		t.Errorf("UnmarshalPacketBody(): Attrs.Flags was %#x, but expected %#x", ep.Attrs.Flags, sshfx.AttrPermissions)
	}

	if ep.Attrs.Permissions != perm {
		t.Errorf("UnmarshalPacketBody(): Attrs.Permissions was %#x, but expected %#x", ep.Attrs.Permissions, perm)
	}
}
