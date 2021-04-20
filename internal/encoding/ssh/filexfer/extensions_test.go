package filexfer

import (
	"bytes"
	"testing"
)

func TestExtensionPair(t *testing.T) {
	const (
		name = "foo"
		data = "1"
	)

	pair := &ExtensionPair{
		Name: name,
		Data: data,
	}

	buf := new(Buffer)

	pair.MarshalInto(buf)

	want := []byte{
		0x00, 0x00, 0x00, 3,
		'f', 'o', 'o',
		0x00, 0x00, 0x00, 1,
		'1',
	}

	if got := buf.Bytes(); !bytes.Equal(got, want) {
		t.Errorf("ExtensionPair.MarshalInto() = %X, but wanted %X", got, want)
	}

	*pair = ExtensionPair{}

	if err := pair.UnmarshalFrom(buf); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if pair.Name != name {
		t.Errorf("ExtensionPair.UnmarshalFrom(): Name was %q, but expected %q", pair.Name, name)
	}

	if pair.Data != data {
		t.Errorf("RawPacket.UnmarshalBinary(): Data was %q, but expected %q", pair.Data, data)
	}

}
