package filexfer

import (
	"bytes"
	"testing"
)

func TestRawPacket(t *testing.T) {
	const (
		id   = 42
		path = "foo"
	)

	p := &RawPacket{
		Type:      PacketTypeStat,
		RequestID: id,
		Data: Buffer{
			b: []byte{0x00, 0x00, 0x00, 0x03, 'f', 'o', 'o'},
		},
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 12,
		17,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 3, 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Errorf("RawPacket.Marshal() = %X, but wanted %X", data, want)
	}

	*p = RawPacket{}

	if err := p.ReadFrom(bytes.NewReader(data), nil); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.Type != PacketTypeStat {
		t.Errorf("RawPacket.UnmarshalBinary(): Type was %v, but expected %v", p.Type, PacketTypeStat)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("RawPacket.UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	want = []byte{
		0x00, 0x00, 0x00, 3, 'f', 'o', 'o',
	}

	if !bytes.Equal(p.Data.Bytes(), want) {
		t.Errorf("RawPacket.UnmarshalBinary(): Data was %X, but expected %X", p.Data, want)
	}

	rp, err := p.RequestPacket()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	if rp.RequestID != uint32(id) {
		t.Errorf("RawPacket.RequestPacket(): RequestID was %d, but expected %d", rp.RequestID, id)
	}

	req, ok := rp.Request.(*StatPacket)
	if !ok {
		t.Fatalf("unexpected Request type was %T, but expected %T", rp.Request, req)
	}

	if req.Path != path {
		t.Errorf("RawPacket.RequestPacket(): Request.Path was %q, but expected %q", req.Path, path)
	}
}

func TestRequestPacket(t *testing.T) {
	const (
		id   = 42
		path = "foo"
	)

	p := &RequestPacket{
		RequestID: id,
		Request: &StatPacket{
			Path: path,
		},
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 12,
		17,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 3, 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Errorf("RequestPacket.Marshal() = %X, but wanted %X", data, want)
	}

	*p = RequestPacket{}

	if err := p.ReadFrom(bytes.NewReader(data), nil); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("RequestPacket.UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	req, ok := p.Request.(*StatPacket)
	if !ok {
		t.Fatalf("unexpected Request type was %T, but expected %T", p.Request, req)
	}

	if req.Path != path {
		t.Errorf("RequestPacket.UnmarshalBinary(): Request.Path was %q, but expected %q", req.Path, path)
	}
}
