package sshfx

import (
	"bytes"
	"testing"
)

// packetBody returns the encoding of p without the uint32(length),
// which is what RawPacket.UnmarshalBinary and RequestPacket.UnmarshalBinary expect.
func packetBody(tb testing.TB, p PacketMarshaller) []byte {
	tb.Helper()

	data, err := ComposePacket(p.MarshalPacket(42, nil))
	if err != nil {
		tb.Fatal(err)
	}

	// The unmarshallers expect the packet type, but not ComposePacket's length prefix.
	return data[4:]
}

var fuzzAttrs = Attributes{
	Flags:       AttrSize | AttrUIDGID | AttrPermissions | AttrACModTime | AttrExtended,
	Size:        1 << 40,
	UID:         1000,
	GID:         100,
	Permissions: ModeRegular | 0o644,
	ATime:       1,
	MTime:       2,
	Extended:    []ExtendedAttribute{{Type: "foo", Data: "bar"}},
}

// FuzzRequestPacket decodes request packets the way the server does,
// and checks that whatever decodes can be encoded and decoded again unchanged.
func FuzzRequestPacket(f *testing.F) {
	packets := []PacketMarshaller{
		&OpenPacket{Filename: "/foo", PFlags: FlagRead | FlagWrite | FlagCreate, Attrs: fuzzAttrs},
		&WritePacket{Handle: "1", Offset: 1 << 40, Data: []byte("data")},
		&ReadPacket{Handle: "1", Offset: 1 << 40, Length: 1 << 15},
		&SetStatPacket{Path: "/foo", Attrs: fuzzAttrs},
		&RenamePacket{OldPath: "/foo", NewPath: "/bar"},
		&ReadDirPacket{Handle: "2"},
		&ExtendedPacket{ExtendedRequest: "foo@example.com", Data: NewBuffer([]byte("data"))},
	}

	for _, p := range packets {
		f.Add(packetBody(f, p))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var p RequestPacket
		if err := p.UnmarshalBinary(data); err != nil {
			return
		}

		first, err := p.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary() of decoded %T: %v", p.Request, err)
		}

		var p2 RequestPacket
		if err := p2.UnmarshalBinary(first[4:]); err != nil {
			t.Fatalf("UnmarshalBinary() of re-encoded %T: %v\n%X", p.Request, err, first)
		}

		second, err := p2.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary() of re-decoded %T: %v", p2.Request, err)
		}

		if !bytes.Equal(first, second) {
			t.Fatalf("%T does not survive a round trip:\n first: %X\nsecond: %X", p.Request, first, second)
		}
	})
}

// FuzzPacketBody decodes packets of any type the way the client does for responses,
// and checks that whatever decodes can be encoded and decoded again unchanged.
func FuzzPacketBody(f *testing.F) {
	packets := []PacketMarshaller{
		&StatusPacket{StatusCode: StatusNoSuchFile, ErrorMessage: "no such file", LanguageTag: "en"},
		&HandlePacket{Handle: "1"},
		&DataPacket{Data: []byte("data")},
		&NamePacket{Entries: []*NameEntry{
			{Filename: "foo", Longname: "-rw-r--r-- 1 user group 3 Jan 1 00:00 foo", Attrs: fuzzAttrs},
			{Filename: "bar"},
		}},
		&AttrsPacket{Attrs: fuzzAttrs},
		&ExtendedReplyPacket{Data: NewBuffer([]byte("data"))},
	}

	for _, p := range packets {
		f.Add(packetBody(f, p))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		var raw RawPacket
		if err := raw.UnmarshalBinary(data); err != nil {
			return
		}

		body, err := raw.PacketBody()
		if err != nil {
			return
		}

		first, err := ComposePacket(body.MarshalPacket(raw.RequestID, nil))
		if err != nil {
			t.Fatalf("MarshalPacket() of decoded %T: %v", body, err)
		}

		var raw2 RawPacket
		if err := raw2.UnmarshalBinary(first[4:]); err != nil {
			t.Fatalf("UnmarshalBinary() of re-encoded %T: %v\n%X", body, err, first)
		}

		body2, err := raw2.PacketBody()
		if err != nil {
			t.Fatalf("PacketBody() of re-encoded %T: %v\n%X", body, err, first)
		}

		second, err := ComposePacket(body2.MarshalPacket(raw2.RequestID, nil))
		if err != nil {
			t.Fatalf("MarshalPacket() of re-decoded %T: %v", body2, err)
		}

		if !bytes.Equal(first, second) {
			t.Fatalf("%T does not survive a round trip:\n first: %X\nsecond: %X", body, first, second)
		}
	})
}

// FuzzVersionPacket decodes the version packet the client receives during the handshake.
func FuzzVersionPacket(f *testing.F) {
	seed, err := (&VersionPacket{
		Version: 3,
		Extensions: []*ExtensionPair{
			{Name: "posix-rename@openssh.com", Data: "1"},
			{Name: "statvfs@openssh.com", Data: "2"},
		},
	}).MarshalBinary()
	if err != nil {
		f.Fatal(err)
	}

	// Skip the uint32(length) and uint8(type), as UnmarshalBinary expects.
	f.Add(seed[5:])

	f.Fuzz(func(t *testing.T, data []byte) {
		var p VersionPacket
		if err := p.UnmarshalBinary(data); err != nil {
			return
		}

		first, err := p.MarshalBinary()
		if err != nil {
			t.Fatal("MarshalBinary():", err)
		}

		var p2 VersionPacket
		if err := p2.UnmarshalBinary(first[5:]); err != nil {
			t.Fatalf("UnmarshalBinary() of re-encoded packet: %v\n%X", err, first)
		}

		second, err := p2.MarshalBinary()
		if err != nil {
			t.Fatal("MarshalBinary():", err)
		}

		if !bytes.Equal(first, second) {
			t.Fatalf("VersionPacket does not survive a round trip:\n first: %X\nsecond: %X", first, second)
		}
	})
}
