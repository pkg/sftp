package sftp

import "testing"
import "bytes"

var marshalUint32Tests = []struct {
	v    uint32
	want []byte
}{
	{1, []byte{0, 0, 0, 1}},
	{256, []byte{0, 0, 1, 0}},
	{^uint32(0), []byte{255, 255, 255, 255}},
}

func TestMarshalUint32(t *testing.T) {
	for _, tt := range marshalUint32Tests {
		got := marshalUint32(nil, tt.v)
		if !bytes.Equal(tt.want, got) {
			t.Errorf("marshalUint32(%d): want %v, got %v", tt.v, tt.want, got)
		}
	}
}

var marshalUint64Tests = []struct {
	v    uint64
	want []byte
}{
	{1, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1}},
	{256, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0}},
	{^uint64(0), []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
	{1 << 32, []byte{0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0}},
}

func TestMarshalUint64(t *testing.T) {
	for _, tt := range marshalUint64Tests {
		got := marshalUint64(nil, tt.v)
		if !bytes.Equal(tt.want, got) {
			t.Errorf("marshalUint64(%d): want %#v, got %#v", tt.v, tt.want, got)
		}
	}
}

var marshalStringTests = []struct {
	v    string
	want []byte
}{
	{"", []byte{0, 0, 0, 0}},
	{"/foo", []byte{0x0, 0x0, 0x0, 0x4, 0x2f, 0x66, 0x6f, 0x6f}},
}

func TestMarshalString(t *testing.T) {
	for _, tt := range marshalStringTests {
		got := marshalString(nil, tt.v)
		if !bytes.Equal(tt.want, got) {
			t.Errorf("marshalString(%q): want %#v, got %#v", tt.v, tt.want, got)
		}
	}
}
