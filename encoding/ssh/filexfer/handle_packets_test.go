package filexfer

import (
	"bytes"
	"testing"
)

func TestClosePacket(t *testing.T) {
	const (
		id     = 42
		handle = "somehandle"
	)

	p := &ClosePacket{
		RequestID: id,
		Handle:    "somehandle",
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 19,
		4,
		0x00, 0x00, 0x00, id,
		0x00, 0x00, 0x00, 10, 's', 'o', 'm', 'e', 'h', 'a', 'n', 'd', 'l', 'e',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = ClosePacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.Handle != handle {
		t.Fatalf("UnmarshalBinary(): Handle was %q, but expected %q", p.Handle, handle)
	}
}

func TestReadPacket(t *testing.T) {
	const (
		id     = 42
		handle = "somehandle"
		offset = 0x123456789ABCDEF0
		length = 0xFEDCBA98
	)

	p := &ReadPacket{
		RequestID: id,
		Handle:    "somehandle",
		Offset:    offset,
		Len:       length,
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 31,
		5,
		0x00, 0x00, 0x00, id,
		0x00, 0x00, 0x00, 10, 's', 'o', 'm', 'e', 'h', 'a', 'n', 'd', 'l', 'e',
		0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0,
		0xFE, 0xDC, 0xBA, 0x98,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = ReadPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.Handle != handle {
		t.Fatalf("UnmarshalBinary(): Handle was %q, but expected %q", p.Handle, handle)
	}

	if p.Offset != offset {
		t.Fatalf("UnmarshalBinary(): Offset was %x, but expected %x", p.Offset, offset)
	}

	if p.Len != length {
		t.Fatalf("UnmarshalBinary(): Len was %x, but expected %x", p.Len, length)
	}
}

func TestWritePacket(t *testing.T) {
	const (
		id     = 42
		handle = "somehandle"
		offset = 0x123456789ABCDEF0
	)

	var payload = []byte(`foobar`)

	p := &WritePacket{
		RequestID: id,
		Handle:    "somehandle",
		Offset:    offset,
		Data:      payload,
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 37,
		6,
		0x00, 0x00, 0x00, id,
		0x00, 0x00, 0x00, 10, 's', 'o', 'm', 'e', 'h', 'a', 'n', 'd', 'l', 'e',
		0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0,
		0x00, 0x00, 0x00, 0x06, 'f', 'o', 'o', 'b', 'a', 'r',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = WritePacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.Handle != handle {
		t.Fatalf("UnmarshalBinary(): Handle was %q, but expected %q", p.Handle, handle)
	}

	if p.Offset != offset {
		t.Fatalf("UnmarshalBinary(): Offset was %x, but expected %x", p.Offset, offset)
	}

	if !bytes.Equal(p.Data, payload) {
		t.Fatalf("UnmarshalBinary(): Data was %X, but expected %X", p.Data, payload)
	}
}

func TestFstatPacket(t *testing.T) {
	const (
		id     = 42
		handle = "somehandle"
		flags  = 0x12345678
	)

	p := &FstatPacket{
		RequestID: id,
		Handle:    "somehandle",
		Flags:     flags,
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 23,
		8,
		0x00, 0x00, 0x00, id,
		0x00, 0x00, 0x00, 10, 's', 'o', 'm', 'e', 'h', 'a', 'n', 'd', 'l', 'e',
		0x12, 0x34, 0x56, 0x78,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = FstatPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.Handle != handle {
		t.Fatalf("UnmarshalBinary(): Handle was %q, but expected %q", p.Handle, handle)
	}

	if p.Flags != flags {
		t.Fatalf("UnmarshalBinary(): Flags was %x, but expected %x", p.Flags, flags)
	}
}

func TestFsetstatPacket(t *testing.T) {
	const (
		id     = 42
		handle = "somehandle"
		perms  = 0x12345678
	)

	p := &FsetstatPacket{
		RequestID: id,
		Handle:    "somehandle",
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
		0x00, 0x00, 0x00, 27,
		10,
		0x00, 0x00, 0x00, id,
		0x00, 0x00, 0x00, 10, 's', 'o', 'm', 'e', 'h', 'a', 'n', 'd', 'l', 'e',
		0x00, 0x00, 0x00, 0x04,
		0x12, 0x34, 0x56, 0x78,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = FsetstatPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.Handle != handle {
		t.Fatalf("UnmarshalBinary(): Handle was %q, but expected %q", p.Handle, handle)
	}
}

func TestReaddirPacket(t *testing.T) {
	const (
		id     = 42
		handle = "somehandle"
	)

	p := &ReaddirPacket{
		RequestID: id,
		Handle:    "somehandle",
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 19,
		12,
		0x00, 0x00, 0x00, id,
		0x00, 0x00, 0x00, 10, 's', 'o', 'm', 'e', 'h', 'a', 'n', 'd', 'l', 'e',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = ReaddirPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.Handle != handle {
		t.Fatalf("UnmarshalBinary(): Handle was %q, but expected %q", p.Handle, handle)
	}
}
