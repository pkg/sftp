package sshfx

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"testing"
)

func TestStatusPacketIs(t *testing.T) {
	status := &StatusPacket{
		StatusCode:   StatusFailure,
		ErrorMessage: "error message",
		LanguageTag:  "language tag",
	}

	diffCode := new(StatusPacket)
	*diffCode = *status
	diffCode.StatusCode = StatusOpUnsupported

	diffMsg := new(StatusPacket)
	*diffMsg = *status
	diffMsg.ErrorMessage = "foo"

	diffLang := new(StatusPacket)
	*diffLang = *status
	diffLang.LanguageTag = "bar"

	if errors.Is(status, diffCode) {
		t.Error("errors.Is(status, diffCode) should be false")
	}
	if errors.Is(status, diffMsg) {
		t.Error("errors.Is(status, diffMsg) should be false")
	}
	if errors.Is(status, diffLang) {
		t.Error("errors.Is(status, diffLang) should be false")
	}

	if !errors.Is(status, StatusFailure) {
		t.Error("errors.Is(status, StatusFailure) should be true")
	}
	if errors.Is(status, StatusOK) {
		t.Error("errors.Is(status, StatusOK) should not be true")
	}

	/*// Neither of these tests make sense to test, as their behavior is dictated by errors.Is.
	if errors.Is(status, nil) {
		t.Error("errors.Is(status, nil) should not be true")
	}

	myOK := &StatusPacket{
		StatusCode: StatusOK,
		ErrorMessage: "error message",
		LanguageTag: "language tag",
	}

	if !errors.Is(myOK, nil) {
		t.Error("errors.Is(myOK, nil) should be true")
	}
	//*/

	myEOF := &StatusPacket{
		StatusCode:   StatusEOF,
		ErrorMessage: "error message",
		LanguageTag:  "language tag",
	}

	if !errors.Is(myEOF, io.EOF) {
		t.Error("errors.Is(myEOF, io.EOF) should be true")
	}

	myNotExist := &StatusPacket{
		StatusCode:   StatusNoSuchFile,
		ErrorMessage: "error message",
		LanguageTag:  "language tag",
	}

	if !errors.Is(myNotExist, fs.ErrNotExist) {
		t.Error("errors.Is(myNotExist, fs.ErrNotExist) should be true")
	}

	myPerm := &StatusPacket{
		StatusCode:   StatusPermissionDenied,
		ErrorMessage: "error message",
		LanguageTag:  "language tag",
	}

	if !errors.Is(myPerm, fs.ErrPermission) {
		t.Error("errors.Is(myPerm, fs.ErrPermission) should be true")
	}
}

var _ Packet = &StatusPacket{}

func TestStatusPacket(t *testing.T) {
	const (
		id           = 42
		statusCode   = StatusBadMessage
		errorMessage = "foo"
		languageTag  = "x-example"
	)

	p := &StatusPacket{
		StatusCode:   statusCode,
		ErrorMessage: errorMessage,
		LanguageTag:  languageTag,
	}

	buf, err := testComposePacket(t, p.MarshalPacket, id, nil)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 29,
		101,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 5,
		0x00, 0x00, 0x00, 3, 'f', 'o', 'o',
		0x00, 0x00, 0x00, 9, 'x', '-', 'e', 'x', 'a', 'm', 'p', 'l', 'e',
	}

	if !bytes.Equal(buf, want) {
		t.Fatalf("MarshalPacket() = %X, but wanted %X", buf, want)
	}

	*p = StatusPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(buf[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.StatusCode != statusCode {
		t.Errorf("UnmarshalBinary(): StatusCode was %v, but expected %v", p.StatusCode, statusCode)
	}

	if p.ErrorMessage != errorMessage {
		t.Errorf("UnmarshalBinary(): ErrorMessage was %q, but expected %q", p.ErrorMessage, errorMessage)
	}

	if p.LanguageTag != languageTag {
		t.Errorf("UnmarshalBinary(): LanguageTag was %q, but expected %q", p.LanguageTag, languageTag)
	}
}

var _ Packet = &HandlePacket{}

func TestHandlePacket(t *testing.T) {
	const (
		id     = 42
		handle = "somehandle"
	)

	p := &HandlePacket{
		Handle: "somehandle",
	}

	buf, err := testComposePacket(t, p.MarshalPacket, id, nil)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 19,
		102,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 10, 's', 'o', 'm', 'e', 'h', 'a', 'n', 'd', 'l', 'e',
	}

	if !bytes.Equal(buf, want) {
		t.Fatalf("MarshalPacket() = %X, but wanted %X", buf, want)
	}

	*p = HandlePacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(buf[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Handle != handle {
		t.Errorf("UnmarshalBinary(): Handle was %q, but expected %q", p.Handle, handle)
	}
}

var _ Packet = &DataPacket{}

func TestDataPacket(t *testing.T) {
	const (
		id = 42
	)

	var payload = []byte(`foobar`)

	p := &DataPacket{
		Data: payload,
	}

	buf, err := testComposePacket(t, p.MarshalPacket, id, nil)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 15,
		103,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 6, 'f', 'o', 'o', 'b', 'a', 'r',
	}

	if !bytes.Equal(buf, want) {
		t.Fatalf("MarshalPacket() = %X, but wanted %X", buf, want)
	}

	*p = DataPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(buf[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if !bytes.Equal(p.Data, payload) {
		t.Errorf("UnmarshalBinary(): Data was %X, but expected %X", p.Data, payload)
	}
}

var _ Packet = &NamePacket{}

func TestNamePacket(t *testing.T) {
	const (
		id                = 42
		filename          = "foo"
		longname          = "bar"
		perms    FileMode = 0x87654300
	)

	p := &NamePacket{
		Entries: []*NameEntry{
			{
				Filename: filename + "1",
				Longname: longname + "1",
				Attrs: Attributes{
					Flags:       AttrPermissions | (1 << 8),
					Permissions: perms | 1,
				},
			},
			{
				Filename: filename + "2",
				Longname: longname + "2",
				Attrs: Attributes{
					Flags:       AttrPermissions | (2 << 8),
					Permissions: perms | 2,
				},
			},
		},
	}

	buf, err := testComposePacket(t, p.MarshalPacket, id, nil)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 57,
		104,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 4, 'f', 'o', 'o', '1',
		0x00, 0x00, 0x00, 4, 'b', 'a', 'r', '1',
		0x00, 0x00, 0x01, 0x04,
		0x87, 0x65, 0x43, 0x01,
		0x00, 0x00, 0x00, 4, 'f', 'o', 'o', '2',
		0x00, 0x00, 0x00, 4, 'b', 'a', 'r', '2',
		0x00, 0x00, 0x02, 0x04,
		0x87, 0x65, 0x43, 0x02,
	}

	if !bytes.Equal(buf, want) {
		t.Fatalf("MarshalPacket() = %X, but wanted %X", buf, want)
	}

	*p = NamePacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(buf[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if count := len(p.Entries); count != 2 {
		t.Fatalf("UnmarshalBinary(): len(NameEntries) was %d, but expected %d", count, 2)
	}

	for i, e := range p.Entries {
		if got, want := e.Filename, filename+string('1'+rune(i)); got != want {
			t.Errorf("UnmarshalBinary(): Entries[%d].Filename was %q, but expected %q", i, got, want)
		}

		if got, want := e.Longname, longname+string('1'+rune(i)); got != want {
			t.Errorf("UnmarshalBinary(): Entries[%d].Longname was %q, but expected %q", i, got, want)
		}

		if got, want := e.Attrs.Flags, AttrPermissions|((i+1)<<8); got != uint32(want) {
			t.Errorf("UnmarshalBinary(): Entries[%d].Attrs.Flags was %#x, but expected %#x", i, got, want)
		}

		if got, want := e.Attrs.Permissions, perms|FileMode(i+1); got != want {
			t.Errorf("UnmarshalBinary(): Entries[%d].Attrs.Permissions was %#v, but expected %#v", i, got, want)
		}
	}
}

var _ Packet = &PathPseudoPacket{}

func TestPathPseudoPacket(t *testing.T) {
	const (
		id       = 42
		filename = "foo"
	)

	p := &PathPseudoPacket{
		Path: filename,
	}

	buf, err := testComposePacket(t, p.MarshalPacket, id, nil)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 16 + 8,
		104,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 3, 'f', 'o', 'o',
		0x00, 0x00, 0x00, 0, // ""
		0x00, 0x00, 0x00, 0x00,
	}

	if !bytes.Equal(buf, want) {
		t.Fatalf("MarshalPacket():\n  actual: %X\nexpected: %X", buf, want)
	}

	*p = PathPseudoPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(buf[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if got, want := p.Path, filename; got != want {
		t.Errorf("UnmarshalBinary(): Path was %q, but expected %q", got, want)
	}
}

var _ Packet = &AttrsPacket{}

func TestAttrsPacket(t *testing.T) {
	const (
		id             = 42
		perms FileMode = 0x87654321
	)

	p := &AttrsPacket{
		Attrs: Attributes{
			Flags:       AttrPermissions,
			Permissions: perms,
		},
	}

	buf, err := testComposePacket(t, p.MarshalPacket, id, nil)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 13,
		105,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 0x04,
		0x87, 0x65, 0x43, 0x21,
	}

	if !bytes.Equal(buf, want) {
		t.Fatalf("MarshalPacket() = %X, but wanted %X", buf, want)
	}

	*p = AttrsPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalPacketBody(NewBuffer(buf[9:])); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Attrs.Flags != AttrPermissions {
		t.Errorf("UnmarshalBinary(): Attrs.Flags was %#x, but expected %#x", p.Attrs.Flags, AttrPermissions)
	}

	if p.Attrs.Permissions != perms {
		t.Errorf("UnmarshalBinary(): Attrs.Permissions was %#v, but expected %#v", p.Attrs.Permissions, perms)
	}
}
