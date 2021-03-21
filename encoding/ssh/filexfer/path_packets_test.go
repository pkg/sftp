package filexfer

import (
	"bytes"
	"testing"
)

var _ Packet = &LstatPacket{}

func TestLstatPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &LstatPacket{
		Path: path,
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 13,
		7,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = LstatPacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Path != path {
		t.Errorf("UnmarshalPacketBody(): Path was %q, but expected %q", p.Path, path)
	}
}

var _ Packet = &SetstatPacket{}

func TestSetstatPacket(t *testing.T) {
	const (
		id    = 42
		path  = "/foo"
		perms = 0x87654321
	)

	p := &SetstatPacket{
		Path: "/foo",
		Attrs: Attributes{
			Flags:       AttrPermissions,
			Permissions: perms,
		},
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 21,
		9,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
		0x00, 0x00, 0x00, 0x04,
		0x87, 0x65, 0x43, 0x21,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = SetstatPacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Path != path {
		t.Errorf("UnmarshalPacketBody(): Path was %q, but expected %q", p.Path, path)
	}

	if p.Attrs.Flags != AttrPermissions {
		t.Errorf("UnmarshalPacketBody(): Attrs.Flags was %#x, but expected %#x", p.Attrs.Flags, AttrPermissions)
	}

	if p.Attrs.Permissions != perms {
		t.Errorf("UnmarshalPacketBody(): Attrs.Permissions was %#x, but expected %#x", p.Attrs.Permissions, perms)
	}
}

var _ Packet = &RemovePacket{}

func TestRemovePacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &RemovePacket{
		Path: path,
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 13,
		13,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = RemovePacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Path != path {
		t.Errorf("UnmarshalPacketBody(): Path was %q, but expected %q", p.Path, path)
	}
}

var _ Packet = &MkdirPacket{}

func TestMkdirPacket(t *testing.T) {
	const (
		id    = 42
		path  = "/foo"
		perms = 0x87654321
	)

	p := &MkdirPacket{
		Path: "/foo",
		Attrs: Attributes{
			Flags:       AttrPermissions,
			Permissions: perms,
		},
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 21,
		14,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
		0x00, 0x00, 0x00, 0x04,
		0x87, 0x65, 0x43, 0x21,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = MkdirPacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Path != path {
		t.Errorf("UnmarshalPacketBody(): Path was %q, but expected %q", p.Path, path)
	}

	if p.Attrs.Flags != AttrPermissions {
		t.Errorf("UnmarshalPacketBody(): Attrs.Flags was %#x, but expected %#x", p.Attrs.Flags, AttrPermissions)
	}

	if p.Attrs.Permissions != perms {
		t.Errorf("UnmarshalPacketBody(): Attrs.Permissions was %#x, but expected %#x", p.Attrs.Permissions, perms)
	}
}

var _ Packet = &RmdirPacket{}

func TestRmdirPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &RmdirPacket{
		Path: path,
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 13,
		15,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = RmdirPacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Path != path {
		t.Errorf("UnmarshalPacketBody(): Path was %q, but expected %q", p.Path, path)
	}
}

var _ Packet = &RealpathPacket{}

func TestRealpathPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &RealpathPacket{
		Path: path,
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 13,
		16,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = RealpathPacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Path != path {
		t.Errorf("UnmarshalPacketBody(): Path was %q, but expected %q", p.Path, path)
	}
}

var _ Packet = &StatPacket{}

func TestStatPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &StatPacket{
		Path: path,
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 13,
		17,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = StatPacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Path != path {
		t.Errorf("UnmarshalPacketBody(): Path was %q, but expected %q", p.Path, path)
	}
}

var _ Packet = &RenamePacket{}

func TestRenamePacket(t *testing.T) {
	const (
		id      = 42
		oldpath = "/foo"
		newpath = "/bar"
	)

	p := &RenamePacket{
		OldPath: oldpath,
		NewPath: newpath,
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 21,
		18,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
		0x00, 0x00, 0x00, 4, '/', 'b', 'a', 'r',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = RenamePacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.OldPath != oldpath {
		t.Errorf("UnmarshalPacketBody(): OldPath was %q, but expected %q", p.OldPath, oldpath)
	}

	if p.NewPath != newpath {
		t.Errorf("UnmarshalPacketBody(): NewPath was %q, but expected %q", p.NewPath, newpath)
	}
}

var _ Packet = &ReadlinkPacket{}

func TestReadlinkPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &ReadlinkPacket{
		Path: path,
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 13,
		19,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = ReadlinkPacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Path != path {
		t.Errorf("UnmarshalPacketBody(): Path was %q, but expected %q", p.Path, path)
	}
}

var _ Packet = &SymlinkPacket{}

func TestSymlinkPacket(t *testing.T) {
	const (
		id         = 42
		linkpath   = "/foo"
		targetpath = "/bar"
	)

	p := &SymlinkPacket{
		LinkPath:   linkpath,
		TargetPath: targetpath,
	}

	data, err := ComposePacket(p.MarshalPacket(id))
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 21,
		20,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'b', 'a', 'r', // Arguments were inadvertently reversed.
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = SymlinkPacket{}

	// UnmarshalPacketBody assumes the (length, type, request-id) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(data[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.LinkPath != linkpath {
		t.Errorf("UnmarshalPacketBody(): LinkPath was %q, but expected %q", p.LinkPath, linkpath)
	}

	if p.TargetPath != targetpath {
		t.Errorf("UnmarshalPacketBody(): TargetPath was %q, but expected %q", p.TargetPath, targetpath)
	}
}
