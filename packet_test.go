package sftp

import (
	"bytes"
	"encoding"
	"errors"
	"io/ioutil"
	"math"
	"os"
	"reflect"
	"strconv"
	"testing"
)

func TestMarshalUint32(t *testing.T) {
	var tests = []struct {
		v    uint32
		want []byte
	}{
		{0, []byte{0, 0, 0, 0}},
		{42, []byte{0, 0, 0, 42}},
		{42 << 8, []byte{0, 0, 42, 0}},
		{42 << 16, []byte{0, 42, 0, 0}},
		{42 << 24, []byte{42, 0, 0, 0}},
		{^uint32(0), []byte{255, 255, 255, 255}},
	}

	for _, tt := range tests {
		got := marshalUint32(nil, tt.v)
		if !bytes.Equal(tt.want, got) {
			t.Errorf("marshalUint32(%d) = %#v, want %#v", tt.v, got, tt.want)
		}
	}
}

func TestUnmarshalFileStatExtendedOverflow(t *testing.T) {
	// flags = EXTENDED only, extended_count = 0xFFFFFFFF, no entries.
	b := marshalUint32(nil, 0xFFFFFFFF)
	if _, _, err := unmarshalFileStat(sshFileXferAttrExtended, b); err != errShortPacket {
		t.Fatalf("expected errShortPacket, got %v", err)
	}

	// Well-formed control: a single extended entry must still parse.
	b = marshalUint32(nil, 1)
	b = marshalString(b, "type")
	b = marshalString(b, "data")
	fs, _, err := unmarshalFileStat(sshFileXferAttrExtended, b)
	if err != nil {
		t.Fatalf("unexpected error parsing valid extended attrs: %v", err)
	}
	if len(fs.Extended) != 1 {
		t.Fatalf("got %d extended attrs, want 1", len(fs.Extended))
	}
	if fs.Extended[0].ExtType != "type" {
		t.Errorf("got ext type %q, want %q", fs.Extended[0].ExtType, "type")
	}
	if fs.Extended[0].ExtData != "data" {
		t.Errorf("got ext data %q, want %q", fs.Extended[0].ExtData, "data")
	}
}

func TestUnmarshalStatusShort(t *testing.T) {
	for _, data := range [][]byte{
		nil,
		{0x00},
		{0x00, 0x00, 0x00, 0x2a}, // only the id, no code
	} {
		if err := unmarshalStatus(42, data); err != errShortPacket {
			t.Errorf("unmarshalStatus(%v): err = %v, want errShortPacket", data, err)
		}
	}
}

func TestUnmarshalDataReply(t *testing.T) {
	// id matches, declared length exceeds the bytes present. The 32-bit
	// overflow of the length field itself is covered by TestUnmarshalCount.
	b := marshalUint32(nil, 1)
	b = marshalUint32(b, 0x7fffffff)
	if _, err := unmarshalDataReply(1, b); err != errShortPacket {
		t.Fatalf("expected errShortPacket, got %v", err)
	}

	// truncated before the id (less than 4 bytes): rejected by unmarshalSID.
	if _, err := unmarshalDataReply(1, []byte{0x00, 0x00}); err != errShortPacket {
		t.Fatalf("expected errShortPacket, got %v", err)
	}

	// truncated before the length field: id present, nothing after it.
	if _, err := unmarshalDataReply(1, marshalUint32(nil, 1)); err != errShortPacket {
		t.Fatalf("expected errShortPacket, got %v", err)
	}

	// mismatched id.
	b = marshalUint32(nil, 2)
	b = marshalUint32(b, 0)
	_, err := unmarshalDataReply(1, b)
	var idErr *unexpectedIDErr
	if !errors.As(err, &idErr) {
		t.Fatalf("err = %v (%T), want *unexpectedIDErr", err, err)
	}
	if idErr.want != 1 {
		t.Errorf("idErr.want = %d, want 1", idErr.want)
	}
	if idErr.got != 2 {
		t.Errorf("idErr.got = %d, want 2", idErr.got)
	}

	// well-formed reply: declared length shorter than the bytes present must
	// return exactly the declared prefix, leaving any trailing bytes out.
	payload := []byte("hello")
	b = marshalUint32(nil, 1)
	b = marshalUint32(b, uint32(len(payload)))
	b = append(b, payload...)
	b = append(b, "extra"...) // trailing bytes beyond the declared length
	got, err := unmarshalDataReply(1, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("got %q, want %q", got, payload)
	}
}

// TestUnmarshalSID exercises the request-id check shared by every client reply
// decoder: a matching id returns the remaining bytes untouched, a mismatch
// reports both ids, and a truncated reply is rejected rather than panicking.
func TestUnmarshalSID(t *testing.T) {
	// matching id: the bytes after the id must be returned verbatim.
	b := marshalUint32(nil, 42)
	b = append(b, 'x', 'y')
	rest, err := unmarshalSID(42, b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(rest, []byte("xy")) {
		t.Errorf("rest = %q, want %q", rest, "xy")
	}

	// matching id with no trailing bytes: rest is empty, not an error.
	rest, err = unmarshalSID(7, marshalUint32(nil, 7))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rest) != 0 {
		t.Errorf("rest = % x, want empty", rest)
	}

	// mismatched id: error carries both the expected and the received id.
	_, err = unmarshalSID(1, marshalUint32(nil, 2))
	var idErr *unexpectedIDErr
	if !errors.As(err, &idErr) {
		t.Fatalf("err = %v (%T), want *unexpectedIDErr", err, err)
	}
	if idErr.want != 1 {
		t.Errorf("idErr.want = %d, want 1", idErr.want)
	}
	if idErr.got != 2 {
		t.Errorf("idErr.got = %d, want 2", idErr.got)
	}

	// truncated before the id is fully present.
	for _, b := range [][]byte{nil, {0x00}, {0x00, 0x00, 0x00}} {
		if _, err := unmarshalSID(1, b); err != errShortPacket {
			t.Errorf("unmarshalSID(%v): err = %v, want errShortPacket", b, err)
		}
	}
}

// TestUnmarshalCount exercises the count decoder used for the SSH_FXP_NAME and
// SSH_FXP_DATA reply lengths. It returns the count and the remaining bytes, and
// guards the 32-bit overflow case that would otherwise produce a negative
// length in make/slice.
func TestUnmarshalCount(t *testing.T) {
	// valid count: trailing bytes are returned untouched.
	b := marshalUint32(nil, 3)
	b = append(b, 'a', 'b', 'c')
	count, rest, err := unmarshalCount(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
	if !bytes.Equal(rest, []byte("abc")) {
		t.Errorf("rest = %q, want %q", rest, "abc")
	}

	// boundary: math.MaxInt32 (0x7fffffff) is the largest value that fits a
	// non-negative int everywhere, so it must be accepted — this guards the
	// off-by-one in the int(v) < 0 check.
	count, _, err = unmarshalCount(marshalUint32(nil, 0x7fffffff))
	if err != nil {
		t.Fatalf("MaxInt32: unexpected error: %v", err)
	}
	if count != math.MaxInt32 {
		t.Errorf("MaxInt32: count = %d, want %d", count, math.MaxInt32)
	}

	// truncated before the count is fully present.
	for _, b := range [][]byte{nil, {0x00}, {0x00, 0x00, 0x00}} {
		if _, _, err := unmarshalCount(b); err != errShortPacket {
			t.Errorf("unmarshalCount(%v): err = %v, want errShortPacket", b, err)
		}
	}

	// A count larger than math.MaxInt32 does not fit in a 32-bit int. On those
	// platforms it must be rejected with errLongPacket; on 64-bit platforms int
	// is wide enough and the value is returned as-is. This check is always true
	// on 64-bit and only ever trips on 32-bit.
	big := marshalUint32(nil, 0x80000000) // math.MaxInt32 + 1, as a uint32
	count, _, err = unmarshalCount(big)
	if strconv.IntSize == 32 {
		if err != errLongPacket {
			t.Errorf("32-bit: err = %v, want errLongPacket", err)
		}
	} else {
		if err != nil {
			t.Fatalf("64-bit: unexpected error: %v", err)
		}
		// int64 throughout: math.MaxInt32+1 overflows int on 32-bit at compile
		// time, even though this branch only runs on 64-bit.
		if int64(count) != int64(math.MaxInt32)+1 {
			t.Errorf("64-bit: count = %d, want %d", count, int64(math.MaxInt32)+1)
		}
	}
}

func TestUnmarshalStringSafe(t *testing.T) {
	// well-formed: the declared bytes are returned as the string and any
	// trailing bytes are handed back untouched as the remainder.
	b := marshalString(nil, "hello")
	b = append(b, 0xde, 0xad)
	s, rest, err := unmarshalStringSafe(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "hello" {
		t.Errorf("s = %q, want %q", s, "hello")
	}
	if !bytes.Equal(rest, []byte{0xde, 0xad}) {
		t.Errorf("rest = % x, want de ad", rest)
	}

	// empty string: a zero length yields "" and leaves the remainder untouched.
	empty := append(marshalUint32(nil, 0), 'x', 'y')
	s, rest, err = unmarshalStringSafe(empty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "" {
		t.Errorf("s = %q, want empty", s)
	}
	if !bytes.Equal(rest, []byte("xy")) {
		t.Errorf("rest = %q, want %q", rest, "xy")
	}

	// exact length: the declared bytes consume the whole slice, empty remainder.
	s, rest, err = unmarshalStringSafe(marshalString(nil, "abc"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "abc" {
		t.Errorf("s = %q, want %q", s, "abc")
	}
	if len(rest) != 0 {
		t.Errorf("rest = % x, want empty", rest)
	}

	// truncated before the length prefix is fully present.
	for _, b := range [][]byte{nil, {0x00}, {0x00, 0x00, 0x00}} {
		if _, _, err := unmarshalStringSafe(b); err != errShortPacket {
			t.Errorf("unmarshalStringSafe(% x): err = %v, want errShortPacket", b, err)
		}
	}

	// declared length exceeds the bytes actually present. The 32-bit overflow
	// of the length field itself is covered by TestUnmarshalCount.
	short := marshalUint32(nil, 5)
	short = append(short, 'a', 'b') // only 2 of the 5 declared bytes
	if _, _, err := unmarshalStringSafe(short); err != errShortPacket {
		t.Errorf("unmarshalStringSafe(short): err = %v, want errShortPacket", err)
	}
}

func TestMarshalUint64(t *testing.T) {
	var tests = []struct {
		v    uint64
		want []byte
	}{
		{0, []byte{0, 0, 0, 0, 0, 0, 0, 0}},
		{42, []byte{0, 0, 0, 0, 0, 0, 0, 42}},
		{42 << 8, []byte{0, 0, 0, 0, 0, 0, 42, 0}},
		{42 << 16, []byte{0, 0, 0, 0, 0, 42, 0, 0}},
		{42 << 24, []byte{0, 0, 0, 0, 42, 0, 0, 0}},
		{42 << 32, []byte{0, 0, 0, 42, 0, 0, 0, 0}},
		{42 << 40, []byte{0, 0, 42, 0, 0, 0, 0, 0}},
		{42 << 48, []byte{0, 42, 0, 0, 0, 0, 0, 0}},
		{42 << 56, []byte{42, 0, 0, 0, 0, 0, 0, 0}},
		{^uint64(0), []byte{255, 255, 255, 255, 255, 255, 255, 255}},
	}

	for _, tt := range tests {
		got := marshalUint64(nil, tt.v)
		if !bytes.Equal(tt.want, got) {
			t.Errorf("marshalUint64(%d) = %#v, want %#v", tt.v, got, tt.want)
		}
	}
}

func TestMarshalString(t *testing.T) {
	var tests = []struct {
		v    string
		want []byte
	}{
		{"", []byte{0, 0, 0, 0}},
		{"/", []byte{0x0, 0x0, 0x0, 0x01, '/'}},
		{"/foo", []byte{0x0, 0x0, 0x0, 0x4, '/', 'f', 'o', 'o'}},
		{"\x00bar", []byte{0x0, 0x0, 0x0, 0x4, 0, 'b', 'a', 'r'}},
		{"b\x00ar", []byte{0x0, 0x0, 0x0, 0x4, 'b', 0, 'a', 'r'}},
		{"ba\x00r", []byte{0x0, 0x0, 0x0, 0x4, 'b', 'a', 0, 'r'}},
		{"bar\x00", []byte{0x0, 0x0, 0x0, 0x4, 'b', 'a', 'r', 0}},
	}

	for _, tt := range tests {
		got := marshalString(nil, tt.v)
		if !bytes.Equal(tt.want, got) {
			t.Errorf("marshalString(%q) = %#v, want %#v", tt.v, got, tt.want)
		}
	}
}

func TestMarshal(t *testing.T) {
	type Struct struct {
		X, Y, Z uint32
	}

	var tests = []struct {
		v    any
		want []byte
	}{
		{uint8(42), []byte{42}},
		{uint32(42 << 8), []byte{0, 0, 42, 0}},
		{uint64(42 << 32), []byte{0, 0, 0, 42, 0, 0, 0, 0}},
		{"foo", []byte{0x0, 0x0, 0x0, 0x3, 'f', 'o', 'o'}},
		{Struct{1, 2, 3}, []byte{0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x3}},
		{[]uint32{1, 2, 3}, []byte{0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x2, 0x0, 0x0, 0x0, 0x3}},
	}

	for _, tt := range tests {
		got := marshal(nil, tt.v)
		if !bytes.Equal(tt.want, got) {
			t.Errorf("marshal(%#v) = %#v, want %#v", tt.v, got, tt.want)
		}
	}
}

func TestUnmarshalUint32(t *testing.T) {
	testBuffer := []byte{
		0, 0, 0, 0,
		0, 0, 0, 42,
		0, 0, 42, 0,
		0, 42, 0, 0,
		42, 0, 0, 0,
		255, 0, 0, 254,
	}

	var wants = []uint32{
		0,
		42,
		42 << 8,
		42 << 16,
		42 << 24,
		255<<24 | 254,
	}

	var i int
	for len(testBuffer) > 0 {
		got, rest := unmarshalUint32(testBuffer)

		if got != wants[i] {
			t.Fatalf("unmarshalUint32(%#v) = %d, want %d", testBuffer[:4], got, wants[i])
		}

		i++
		testBuffer = rest
	}
}

func TestUnmarshalUint64(t *testing.T) {
	testBuffer := []byte{
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 42,
		0, 0, 0, 0, 0, 0, 42, 0,
		0, 0, 0, 0, 0, 42, 0, 0,
		0, 0, 0, 0, 42, 0, 0, 0,
		0, 0, 0, 42, 0, 0, 0, 0,
		0, 0, 42, 0, 0, 0, 0, 0,
		0, 42, 0, 0, 0, 0, 0, 0,
		42, 0, 0, 0, 0, 0, 0, 0,
		255, 0, 0, 0, 0, 0, 0, 254,
	}

	var wants = []uint64{
		0,
		42,
		42 << 8,
		42 << 16,
		42 << 24,
		42 << 32,
		42 << 40,
		42 << 48,
		42 << 56,
		255<<56 | 254,
	}

	var i int
	for len(testBuffer) > 0 {
		got, rest := unmarshalUint64(testBuffer)

		if got != wants[i] {
			t.Fatalf("unmarshalUint64(%#v) = %d, want %d", testBuffer[:8], got, wants[i])
		}

		i++
		testBuffer = rest
	}
}

var unmarshalStringTests = []struct {
	b    []byte
	want string
	rest []byte
}{
	{marshalString(nil, ""), "", nil},
	{marshalString(nil, "blah"), "blah", nil},
}

func TestUnmarshalString(t *testing.T) {
	testBuffer := []byte{
		0, 0, 0, 0,
		0, 0, 0, 1, '/',
		0, 0, 0, 4, '/', 'f', 'o', 'o',
		0, 0, 0, 4, 0, 'b', 'a', 'r',
		0, 0, 0, 4, 'b', 0, 'a', 'r',
		0, 0, 0, 4, 'b', 'a', 0, 'r',
		0, 0, 0, 4, 'b', 'a', 'r', 0,
	}

	var wants = []string{
		"",
		"/",
		"/foo",
		"\x00bar",
		"b\x00ar",
		"ba\x00r",
		"bar\x00",
	}

	var i int
	for len(testBuffer) > 0 {
		got, rest := unmarshalString(testBuffer)

		if got != wants[i] {
			t.Fatalf("unmarshalUint64(%#v...) = %q, want %q", testBuffer[:4], got, wants[i])
		}

		i++
		testBuffer = rest
	}
}

func TestUnmarshalAttrs(t *testing.T) {
	var tests = []struct {
		b    []byte
		want *FileStat
	}{
		{
			b:    []byte{0x00, 0x00, 0x00, 0x00},
			want: &FileStat{},
		},
		{
			b: []byte{
				0x00, 0x00, 0x00, byte(sshFileXferAttrSize),
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 20,
			},
			want: &FileStat{
				Size: 20,
			},
		},
		{
			b: []byte{
				0x00, 0x00, 0x00, byte(sshFileXferAttrSize | sshFileXferAttrPermissions),
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 20,
				0x00, 0x00, 0x01, 0xA4,
			},
			want: &FileStat{
				Size: 20,
				Mode: 0644,
			},
		},
		{
			b: []byte{
				0x00, 0x00, 0x00, byte(sshFileXferAttrSize | sshFileXferAttrPermissions | sshFileXferAttrUIDGID),
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 20,
				0x00, 0x00, 0x03, 0xE8,
				0x00, 0x00, 0x03, 0xE9,
				0x00, 0x00, 0x01, 0xA4,
			},
			want: &FileStat{
				Size: 20,
				Mode: 0644,
				UID:  1000,
				GID:  1001,
			},
		},
		{
			b: []byte{
				0x00, 0x00, 0x00, byte(sshFileXferAttrSize | sshFileXferAttrPermissions | sshFileXferAttrUIDGID | sshFileXferAttrACmodTime),
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 20,
				0x00, 0x00, 0x03, 0xE8,
				0x00, 0x00, 0x03, 0xE9,
				0x00, 0x00, 0x01, 0xA4,
				0x00, 0x00, 0x00, 42,
				0x00, 0x00, 0x00, 13,
			},
			want: &FileStat{
				Size:  20,
				Mode:  0644,
				UID:   1000,
				GID:   1001,
				Atime: 42,
				Mtime: 13,
			},
		},
	}

	for _, tt := range tests {
		got, _, err := unmarshalAttrs(tt.b)
		if err != nil {
			t.Fatal("unexpected error:", err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("unmarshalAttrs(% X):\n-  got: %#v\n- want: %#v", tt.b, got, tt.want)
		}
	}
}

func TestUnmarshalStatus(t *testing.T) {
	var requestID uint32 = 1

	id := marshalUint32(nil, requestID)
	idCode := marshalUint32(id, sshFxFailure)
	idCodeMsg := marshalString(idCode, "err msg")
	idCodeMsgLang := marshalString(idCodeMsg, "lang tag")

	var tests = []struct {
		desc   string
		reqID  uint32
		status []byte
		want   error
	}{
		{
			desc:   "well-formed status",
			status: idCodeMsgLang,
			want: &StatusError{
				Code: sshFxFailure,
				msg:  "err msg",
				lang: "lang tag",
			},
		},
		{
			desc:   "missing language tag",
			status: idCodeMsg,
			want: &StatusError{
				Code: sshFxFailure,
				msg:  "err msg",
			},
		},
		{
			desc:   "missing error message and language tag",
			status: idCode,
			want: &StatusError{
				Code: sshFxFailure,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := unmarshalStatus(1, tt.status)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unmarshalStatus(1, % X):\n-  got: %#v\n- want: %#v", tt.status, got, tt.want)
			}
		})
	}

	got := unmarshalStatus(2, idCodeMsgLang)
	want := &unexpectedIDErr{
		want: 2,
		got:  1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("unmarshalStatus(2, % X):\n-  got: %#v\n- want: %#v", idCodeMsgLang, got, want)
	}
}

func TestSendPacket(t *testing.T) {
	var tests = []struct {
		packet encoding.BinaryMarshaler
		want   []byte
	}{
		{
			packet: &sshFxInitPacket{
				Version: 3,
				Extensions: []extensionPair{
					{"posix-rename@openssh.com", "1"},
				},
			},
			want: []byte{
				0x0, 0x0, 0x0, 0x26,
				0x1,
				0x0, 0x0, 0x0, 0x3,
				0x0, 0x0, 0x0, 0x18,
				'p', 'o', 's', 'i', 'x', '-', 'r', 'e', 'n', 'a', 'm', 'e', '@', 'o', 'p', 'e', 'n', 's', 's', 'h', '.', 'c', 'o', 'm',
				0x0, 0x0, 0x0, 0x1,
				'1',
			},
		},
		{
			packet: &sshFxpOpenPacket{
				ID:     1,
				Path:   "/foo",
				Pflags: toPflags(os.O_RDONLY),
			},
			want: []byte{
				0x0, 0x0, 0x0, 0x15,
				0x3,
				0x0, 0x0, 0x0, 0x1,
				0x0, 0x0, 0x0, 0x4, '/', 'f', 'o', 'o',
				0x0, 0x0, 0x0, 0x1,
				0x0, 0x0, 0x0, 0x0,
			},
		},
		{
			packet: &sshFxpOpenPacket{
				ID:     3,
				Path:   "/foo",
				Pflags: toPflags(os.O_WRONLY | os.O_CREATE | os.O_TRUNC),
				Flags:  sshFileXferAttrPermissions,
				Attrs: &FileStat{
					Mode: 0o755,
				},
			},
			want: []byte{
				0x0, 0x0, 0x0, 0x19,
				0x3,
				0x0, 0x0, 0x0, 0x3,
				0x0, 0x0, 0x0, 0x4, '/', 'f', 'o', 'o',
				0x0, 0x0, 0x0, 0x1a,
				0x0, 0x0, 0x0, 0x4,
				0x0, 0x0, 0x1, 0xed,
			},
		},
		{
			packet: &sshFxpWritePacket{
				ID:     124,
				Handle: "foo",
				Offset: 13,
				Length: uint32(len("bar")),
				Data:   []byte("bar"),
			},
			want: []byte{
				0x0, 0x0, 0x0, 0x1b,
				0x6,
				0x0, 0x0, 0x0, 0x7c,
				0x0, 0x0, 0x0, 0x3, 'f', 'o', 'o',
				0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd,
				0x0, 0x0, 0x0, 0x3, 'b', 'a', 'r',
			},
		},
		{
			packet: &sshFxpSetstatPacket{
				ID:    31,
				Path:  "/bar",
				Flags: sshFileXferAttrUIDGID,
				Attrs: &FileStat{
					UID: 1000,
					GID: 100,
				},
			},
			want: []byte{
				0x0, 0x0, 0x0, 0x19,
				0x9,
				0x0, 0x0, 0x0, 0x1f,
				0x0, 0x0, 0x0, 0x4, '/', 'b', 'a', 'r',
				0x0, 0x0, 0x0, 0x2,
				0x0, 0x0, 0x3, 0xe8,
				0x0, 0x0, 0x0, 0x64,
			},
		},
	}

	for _, tt := range tests {
		b := new(bytes.Buffer)
		sendPacket(b, tt.packet)
		if got := b.Bytes(); !bytes.Equal(tt.want, got) {
			t.Errorf("sendPacket(%v): got %x want %x", tt.packet, tt.want, got)
		}
	}
}

func sp(data encoding.BinaryMarshaler) []byte {
	b := new(bytes.Buffer)
	sendPacket(b, data)
	return b.Bytes()
}

func TestRecvPacket(t *testing.T) {
	var recvPacketTests = []struct {
		b []byte

		want    fxp
		body    []byte
		wantErr error
	}{
		{
			b: sp(&sshFxInitPacket{
				Version: 3,
				Extensions: []extensionPair{
					{"posix-rename@openssh.com", "1"},
				},
			}),
			want: sshFxpInit,
			body: []byte{
				0x0, 0x0, 0x0, 0x3,
				0x0, 0x0, 0x0, 0x18,
				'p', 'o', 's', 'i', 'x', '-', 'r', 'e', 'n', 'a', 'm', 'e', '@', 'o', 'p', 'e', 'n', 's', 's', 'h', '.', 'c', 'o', 'm',
				0x0, 0x0, 0x0, 0x01,
				'1',
			},
		},
		{
			b: []byte{
				0x0, 0x0, 0x0, 0x0,
			},
			wantErr: errShortPacket,
		},
		{
			b: []byte{
				0xff, 0xff, 0xff, 0xff,
			},
			wantErr: errLongPacket,
		},
	}

	for _, tt := range recvPacketTests {
		r := bytes.NewReader(tt.b)

		got, body, err := recvPacket(r, nil, 0)
		if tt.wantErr == nil {
			if err != nil {
				t.Fatalf("recvPacket(%#v): unexpected error: %v", tt.b, err)
			}
		} else {
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("recvPacket(%#v) = %v, want %v", tt.b, err, tt.wantErr)
			}
		}

		if got != tt.want {
			t.Errorf("recvPacket(%#v) = %#v, want %#v", tt.b, got, tt.want)
		}

		if !bytes.Equal(body, tt.body) {
			t.Errorf("recvPacket(%#v) = %#v, want %#v", tt.b, body, tt.body)
		}
	}
}

func TestSSHFxpOpenPacketreadonly(t *testing.T) {
	var tests = []struct {
		pflags uint32
		ok     bool
	}{
		{
			pflags: sshFxfRead,
			ok:     true,
		},
		{
			pflags: sshFxfWrite,
			ok:     false,
		},
		{
			pflags: sshFxfRead | sshFxfWrite,
			ok:     false,
		},
	}

	for _, tt := range tests {
		p := &sshFxpOpenPacket{
			Pflags: tt.pflags,
		}

		if want, got := tt.ok, p.readonly(); want != got {
			t.Errorf("unexpected value for p.readonly(): want: %v, got: %v",
				want, got)
		}
	}
}

func TestSSHFxpOpenPackethasPflags(t *testing.T) {
	var tests = []struct {
		desc      string
		haveFlags uint32
		testFlags []uint32
		ok        bool
	}{
		{
			desc:      "have read, test against write",
			haveFlags: sshFxfRead,
			testFlags: []uint32{sshFxfWrite},
			ok:        false,
		},
		{
			desc:      "have write, test against read",
			haveFlags: sshFxfWrite,
			testFlags: []uint32{sshFxfRead},
			ok:        false,
		},
		{
			desc:      "have read+write, test against read",
			haveFlags: sshFxfRead | sshFxfWrite,
			testFlags: []uint32{sshFxfRead},
			ok:        true,
		},
		{
			desc:      "have read+write, test against write",
			haveFlags: sshFxfRead | sshFxfWrite,
			testFlags: []uint32{sshFxfWrite},
			ok:        true,
		},
		{
			desc:      "have read+write, test against read+write",
			haveFlags: sshFxfRead | sshFxfWrite,
			testFlags: []uint32{sshFxfRead, sshFxfWrite},
			ok:        true,
		},
	}

	for _, tt := range tests {
		t.Log(tt.desc)

		p := &sshFxpOpenPacket{
			Pflags: tt.haveFlags,
		}

		if want, got := tt.ok, p.hasPflags(tt.testFlags...); want != got {
			t.Errorf("unexpected value for p.hasPflags(%#v): want: %v, got: %v",
				tt.testFlags, want, got)
		}
	}
}

func benchMarshal(b *testing.B, packet encoding.BinaryMarshaler) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sendPacket(ioutil.Discard, packet)
	}
}

func BenchmarkMarshalInit(b *testing.B) {
	benchMarshal(b, &sshFxInitPacket{
		Version: 3,
		Extensions: []extensionPair{
			{"posix-rename@openssh.com", "1"},
		},
	})
}

func BenchmarkMarshalOpen(b *testing.B) {
	benchMarshal(b, &sshFxpOpenPacket{
		ID:     1,
		Path:   "/home/test/some/random/path",
		Pflags: toPflags(os.O_RDONLY),
	})
}

func BenchmarkMarshalWriteWorstCase(b *testing.B) {
	data := make([]byte, 32*1024)

	benchMarshal(b, &sshFxpWritePacket{
		ID:     1,
		Handle: "someopaquehandle",
		Offset: 0,
		Length: uint32(len(data)),
		Data:   data,
	})
}

func BenchmarkMarshalWrite1k(b *testing.B) {
	data := make([]byte, 1025)

	benchMarshal(b, &sshFxpWritePacket{
		ID:     1,
		Handle: "someopaquehandle",
		Offset: 0,
		Length: uint32(len(data)),
		Data:   data,
	})
}
