package filexfer

import (
	"bytes"
	"testing"
)

func TestLstatPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &LstatPacket{
		RequestID: id,
		Path:      path,
	}

	data, err := p.MarshalBinary()
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

func TestSetstatPacket(t *testing.T) {
	const (
		id    = 42
		path  = "/foo"
		perms = 0x87654321
	)

	p := &SetstatPacket{
		RequestID: id,
		Path:      "/foo",
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

	if p.Attrs.Flags != AttrPermissions {
		t.Errorf("UnmarshalBinary(): Attrs.Flags was %#x, but expected %#x", p.Attrs.Flags, AttrPermissions)
	}

	if p.Attrs.Permissions != perms {
		t.Errorf("UnmarshalBinary(): Attrs.Permissions was %#x, but expected %#x", p.Attrs.Permissions, perms)
	}
}

func TestRemovePacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &RemovePacket{
		RequestID: id,
		Path:      path,
	}

	data, err := p.MarshalBinary()
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

func TestMkdirPacket(t *testing.T) {
	const (
		id    = 42
		path  = "/foo"
		perms = 0x87654321
	)

	p := &MkdirPacket{
		RequestID: id,
		Path:      "/foo",
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

	if p.Attrs.Flags != AttrPermissions {
		t.Errorf("UnmarshalBinary(): Attrs.Flags was %#x, but expected %#x", p.Attrs.Flags, AttrPermissions)
	}

	if p.Attrs.Permissions != perms {
		t.Errorf("UnmarshalBinary(): Attrs.Permissions was %#x, but expected %#x", p.Attrs.Permissions, perms)
	}
}

func TestRmdirPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &RmdirPacket{
		RequestID: id,
		Path:      path,
	}

	data, err := p.MarshalBinary()
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

func TestRealpathPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &RealpathPacket{
		RequestID: id,
		Path:      path,
	}

	data, err := p.MarshalBinary()
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

func TestStatPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &StatPacket{
		RequestID: id,
		Path:      path,
	}

	data, err := p.MarshalBinary()
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

func TestRenamePacket(t *testing.T) {
	const (
		id      = 42
		oldpath = "/foo"
		newpath = "/bar"
	)

	p := &RenamePacket{
		RequestID: id,
		OldPath:   oldpath,
		NewPath:   newpath,
	}

	data, err := p.MarshalBinary()
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

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.OldPath != oldpath {
		t.Errorf("UnmarshalBinary(): OldPath was %q, but expected %q", p.OldPath, oldpath)
	}

	if p.NewPath != newpath {
		t.Errorf("UnmarshalBinary(): NewPath was %q, but expected %q", p.NewPath, newpath)
	}
}

func TestReadlinkPacket(t *testing.T) {
	const (
		id   = 42
		path = "/foo"
	)

	p := &ReadlinkPacket{
		RequestID: id,
		Path:      path,
	}

	data, err := p.MarshalBinary()
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

func TestSymlinkPacket(t *testing.T) {
	const (
		id         = 42
		linkpath   = "/foo"
		targetpath = "/bar"
	)

	p := &SymlinkPacket{
		RequestID:  id,
		LinkPath:   linkpath,
		TargetPath: targetpath,
	}

	data, err := p.MarshalBinary()
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

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.LinkPath != linkpath {
		t.Errorf("UnmarshalBinary(): LinkPath was %q, but expected %q", p.LinkPath, linkpath)
	}

	if p.TargetPath != targetpath {
		t.Errorf("UnmarshalBinary(): TargetPath was %q, but expected %q", p.TargetPath, targetpath)
	}
}
