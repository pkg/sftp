package filexfer

import (
	"bytes"
	"testing"
)

func TestRawPacket(t *testing.T) {
	var id byte = 42

	p := &RawPacket{
		Type:      PacketTypeStat,
		RequestID: uint32(id),
		Payload: []byte{
			0x00, 0x00, 0x00, 0x03, 'f', 'o', 'o',
		},
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 12,
		17,
		0x00, 0x00, 0x00, id,
		0x00, 0x00, 0x00, 3, 'f', 'o', 'o',
	}

	if !bytes.Equal(data, want) {
		t.Errorf("RawPacket.Marshal() = %X, but wanted %X", data, want)
	}

	*p = RawPacket{}

	// UnmarshalBinary assumes the length has already been consumed.
	if err := p.UnmarshalBinary(data[4:]); err != nil {
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

	if !bytes.Equal(p.Payload, want) {
		t.Errorf("RawPacket.UnmarshalBinary(): Payload was %X, but expected %X", p.Payload, want)
	}
}
