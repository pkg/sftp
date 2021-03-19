package filexfer

import (
	"bytes"
	"testing"
)

func TestOpenPacket(t *testing.T) {
	const (
		id       = 42
		filename = "/foo"
		perms    = 0x87654321
	)

	p := &OpenPacket{
		RequestID: id,
		Filename:  "/foo",
		PFlags:    FlagRead,
		Attrs: Attributes{
			Flags:       AttrPermissions,
			Permissions: perms,
		},
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 25,
		3,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
		0x00, 0x00, 0x00, 1,
		0x00, 0x00, 0x00, 0x04,
		0x87, 0x65, 0x43, 0x21,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = OpenPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.Filename != filename {
		t.Errorf("UnmarshalBinary(): Filename was %q, but expected %q", p.Filename, filename)
	}

	if p.PFlags != FlagRead {
		t.Errorf("UnmarshalBinary(): PFlags was %#x, but expected %#x", p.PFlags, FlagRead)
	}

	if p.Attrs.Flags != AttrPermissions {
		t.Errorf("UnmarshalBinary(): Attrs.Flags was %#x, but expected %#x", p.Attrs.Flags, AttrPermissions)
	}

	if p.Attrs.Permissions != perms {
		t.Errorf("UnmarshalBinary(): Attrs.Permissions was %#x, but expected %#x", p.Attrs.Permissions, perms)
	}
}

func TestOpendirPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &OpendirPacket{
		RequestID: id,
		Path:      path,
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 13,
		11,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = OpendirPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.Path != path {
		t.Errorf("UnmarshalBinary(): Path was %q, but expected %q", p.Path, path)
	}
}
