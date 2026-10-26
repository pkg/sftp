package sftp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kr/fs"
)

// assert that *Client implements fs.FileSystem
var _ fs.FileSystem = new(Client)

// assert that *File implements io.ReadWriteCloser
var _ io.ReadWriteCloser = new(File)

func TestNormaliseError(t *testing.T) {
	var (
		ok         = &StatusError{Code: sshFxOk}
		eof        = &StatusError{Code: sshFxEOF}
		fail       = &StatusError{Code: sshFxFailure}
		noSuchFile = &StatusError{Code: sshFxNoSuchFile}
		foo        = errors.New("foo")
	)

	var tests = []struct {
		desc string
		err  error
		want error
	}{
		{
			desc: "nil error",
		},
		{
			desc: "not *StatusError",
			err:  foo,
			want: foo,
		},
		{
			desc: "*StatusError with ssh_FX_EOF",
			err:  eof,
			want: io.EOF,
		},
		{
			desc: "*StatusError with ssh_FX_NO_SUCH_FILE",
			err:  noSuchFile,
			want: os.ErrNotExist,
		},
		{
			desc: "*StatusError with ssh_FX_OK",
			err:  ok,
		},
		{
			desc: "*StatusError with ssh_FX_FAILURE",
			err:  fail,
			want: fail,
		},
	}

	for _, tt := range tests {
		got := normaliseError(tt.err)
		if got != tt.want {
			t.Errorf("normaliseError(%#v), test %q\n- want: %#v\n-  got: %#v",
				tt.err, tt.desc, tt.want, got)
		}
	}
}

var flagsTests = []struct {
	flags int
	want  uint32
}{
	{os.O_RDONLY, sshFxfRead},
	{os.O_WRONLY, sshFxfWrite},
	{os.O_RDWR, sshFxfRead | sshFxfWrite},
	{os.O_RDWR | os.O_CREATE | os.O_TRUNC, sshFxfRead | sshFxfWrite | sshFxfCreat | sshFxfTrunc},
	{os.O_WRONLY | os.O_APPEND, sshFxfWrite | sshFxfAppend},
}

func TestFlags(t *testing.T) {
	for i, tt := range flagsTests {
		got := toPflags(tt.flags)
		if got != tt.want {
			t.Errorf("test %v: flags(%x): want: %x, got: %x", i, tt.flags, tt.want, got)
		}
	}
}

type packetSizeTest struct {
	size  int
	valid bool
}

var maxPacketCheckedTests = []packetSizeTest{
	{size: 0, valid: false},
	{size: 1, valid: true},
	{size: 32768, valid: true},
	{size: 32769, valid: false},
}

var maxPacketUncheckedTests = []packetSizeTest{
	{size: 0, valid: false},
	{size: 1, valid: true},
	{size: 32768, valid: true},
	{size: 32769, valid: true},
}

func TestMaxPacketChecked(t *testing.T) {
	for _, tt := range maxPacketCheckedTests {
		testMaxPacketOption(t, MaxPacketChecked(tt.size), tt)
	}
}

func TestMaxPacketUnchecked(t *testing.T) {
	for _, tt := range maxPacketUncheckedTests {
		testMaxPacketOption(t, MaxPacketUnchecked(tt.size), tt)
	}
}

func TestMaxPacket(t *testing.T) {
	for _, tt := range maxPacketCheckedTests {
		testMaxPacketOption(t, MaxPacket(tt.size), tt)
	}
}

func testMaxPacketOption(t *testing.T, o ClientOption, tt packetSizeTest) {
	var c Client

	err := o(&c)
	if (err == nil) != tt.valid {
		t.Errorf("MaxPacketChecked(%v)\n- want: %v\n- got: %v", tt.size, tt.valid, err == nil)
	}
	if c.maxPacket != tt.size && tt.valid {
		t.Errorf("MaxPacketChecked(%v)\n- want: %v\n- got: %v", tt.size, tt.size, c.maxPacket)
	}
}

func testFstatOption(t *testing.T, o ClientOption, value bool) {
	var c Client

	err := o(&c)
	if err == nil && c.useFstat != value {
		t.Errorf("UseFStat(%v)\n- want: %v\n- got: %v", value, value, c.useFstat)
	}
}

func TestUseFstatChecked(t *testing.T) {
	testFstatOption(t, UseFstat(true), true)
	testFstatOption(t, UseFstat(false), false)
}

type sink struct{}

func (*sink) Close() error                { return nil }
func (*sink) Write(p []byte) (int, error) { return len(p), nil }

func TestClientZeroLengthPacket(t *testing.T) {
	// Packet length zero (never valid). This used to crash the client.
	packet := []byte{0, 0, 0, 0}

	r := bytes.NewReader(packet)
	c, err := NewClientPipe(r, &sink{})
	if err == nil {
		t.Error("expected an error, got nil")
	}
	if c != nil {
		c.Close()
	}
}

func TestClientShortPacket(t *testing.T) {
	// init packet too short.
	packet := []byte{0, 0, 0, 1, 2}

	r := bytes.NewReader(packet)
	_, err := NewClientPipe(r, &sink{})
	if !errors.Is(err, errShortPacket) {
		t.Fatalf("expected error: %v, got: %v", errShortPacket, err)
	}
}

// frameReply returns a server reply (length prefix + type + body, where body
// begins with the echoed request id) as recvPacket expects it on the wire.
func frameReply(typ fxp, body []byte) []byte {
	b := append([]byte{byte(typ)}, body...)
	return append(marshalUint32(nil, uint32(len(b))), b...)
}

// readRequest reads one length-prefixed request packet from r and returns its
// type and request id. The SSH_FXP_INIT handshake packet carries a version in
// place of an id; callers special-case it on the type.
func readRequest(r io.Reader) (typ fxp, id uint32, ok bool) {
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return 0, 0, false
	}
	n, _ := unmarshalUint32(hdr)
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, 0, false
	}
	if len(body) < 5 {
		return 0, 0, false
	}
	typ = fxp(body[0])
	id, _ = unmarshalUint32(body[1:])
	return typ, id, true
}

// startScriptedClient wires a *Client to a scripted server over synchronized
// io.Pipes. After answering the SSH_FXP_INIT/VERSION handshake, the server
// replies to each request by calling reply(typ, id); reply returns the response
// packet type and body (which must begin with the echoed id). Returning
// ok==false stops the server.
func startScriptedClient(t *testing.T, reply func(typ fxp, id uint32) (fxp, []byte, bool), opts ...ClientOption) *Client {
	t.Helper()

	cr, sw := io.Pipe() // client reads what the server writes
	sr, cw := io.Pipe() // server reads what the client writes

	go func() {
		defer sw.Close()
		for {
			typ, id, ok := readRequest(sr)
			if !ok {
				return
			}
			if typ == sshFxpInit {
				sendPacket(sw, &sshFxVersionPacket{Version: sftpProtocolVersion})
				continue
			}
			rtyp, body, ok := reply(typ, id)
			if !ok {
				return
			}
			if _, err := sw.Write(frameReply(rtyp, body)); err != nil {
				return
			}
		}
	}()

	c, err := NewClientPipe(cr, cw, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClientReadDataReply(t *testing.T) {
	const payload = "ABCDEFGH"

	cases := []struct {
		name    string
		data    func(b []byte) []byte
		wantErr error // nil means the read must succeed and return payload
	}{
		{"valid", func(b []byte) []byte { return marshalString(b, payload) }, nil},
		{"length exceeds bytes present", func(b []byte) []byte { return marshalUint32(b, 0x7fffffff) }, errShortPacket},
		{"length prefix truncated", func(b []byte) []byte { return append(b, 0x00, 0x00) }, errShortPacket},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := startScriptedClient(t, func(_ fxp, id uint32) (fxp, []byte, bool) {
				return sshFxpData, tc.data(marshalUint32(nil, id)), true
			})
			defer c.Close()

			f := &File{c: c, path: "somefile", handle: "somehandle"}
			b := make([]byte, len(payload))
			n, err := f.Read(b)

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				if n != 0 {
					t.Errorf("n = %d, want 0", n)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(b[:n]) != payload {
				t.Errorf("read %q, want %q", b[:n], payload)
			}
		})
	}
}

func TestClientReadChunkAtShortReplies(t *testing.T) {
	for _, reuseChannel := range []bool{false, true} {
		for _, eof := range []bool{false, true} {
			t.Run(fmt.Sprintf("reuseChannel=%t/eof=%t", reuseChannel, eof), func(t *testing.T) {
				var requests int
				c := startScriptedClient(t, func(_ fxp, id uint32) (fxp, []byte, bool) {
					requests++
					if eof && requests > 1 {
						return sshFxpStatus, statusBody(marshalUint32(nil, id), sshFxEOF), true
					}
					return sshFxpData, marshalString(marshalUint32(nil, id), "AB"), true
				})
				defer c.Close()

				var ch chan result
				if reuseChannel {
					ch = make(chan result, 1)
				}
				f := &File{c: c, handle: "somehandle"}
				b := make([]byte, 6)
				n, err := f.readChunkAt(ch, b, 11)
				want, wantErr := "ABABAB", error(nil)
				if eof {
					want, wantErr = "AB", io.EOF
				}
				if !errors.Is(err, wantErr) || n != len(want) || string(b[:n]) != want {
					t.Fatalf("readChunkAt = (%d, %v, %q), want (%d, %v, %q)", n, err, b[:n], len(want), wantErr, want)
				}
			})
		}
	}
}

func TestClientReadAtFirstChunk(t *testing.T) {
	for _, tc := range []struct {
		name    string
		first   string
		eof     bool
		want    string
		wantErr error
	}{
		{name: "short replies", first: "AB", want: "ABABAB"},
		{name: "EOF after first chunk", first: "AB", eof: true, want: "AB", wantErr: io.EOF},
		{name: "initial EOF", eof: true, wantErr: io.EOF},
		{name: "no progress", wantErr: io.ErrNoProgress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests int
			c := startScriptedClient(t, func(_ fxp, id uint32) (fxp, []byte, bool) {
				requests++
				if tc.eof && (tc.first == "" || requests > 1) {
					return sshFxpStatus, statusBody(marshalUint32(nil, id), sshFxEOF), true
				}
				return sshFxpData, marshalString(marshalUint32(nil, id), tc.first), true
			}, MaxPacket(4))
			defer c.Close()

			f := &File{c: c, handle: "somehandle"}
			b := make([]byte, 6)
			n, err := f.ReadAt(b, 11)
			if !errors.Is(err, tc.wantErr) || n != len(tc.want) || string(b[:n]) != tc.want {
				t.Fatalf("ReadAt = (%d, %v, %q), want (%d, %v, %q)", n, err, b[:n], len(tc.want), tc.wantErr, tc.want)
			}
		})
	}
}

func TestClientReadAtConcurrentDataReply(t *testing.T) {
	c := startScriptedClient(t, func(_ fxp, id uint32) (fxp, []byte, bool) {
		// Oversize declared length, no data.
		body := marshalUint32(nil, id)
		body = marshalUint32(body, 0x7fffffff)
		return sshFxpData, body, true
	}, MaxPacket(1024))
	defer c.Close()

	f := &File{c: c, path: "somefile", handle: "somehandle"}
	// A buffer larger than maxPacket forces readAt to split the read into
	// concurrent maxPacket-sized requests.
	n, err := f.ReadAt(make([]byte, 4096), 0)
	if !errors.Is(err, errShortPacket) {
		t.Fatalf("error = %v, want %v", err, errShortPacket)
	}
	if n != 0 {
		t.Errorf("n = %d, want 0", n)
	}
}

// dirEntry appends one well-formed SSH_FXP_NAME entry (filename, longname, and
// empty attributes) to b.
func dirEntry(b []byte, name string) []byte {
	b = marshalString(b, name) // filename
	b = marshalString(b, name) // longname
	return marshalUint32(b, 0) // attrs: flags == 0, no fields follow
}

// statusBody appends an SSH_FXP_STATUS body (code plus empty message/language)
// after an already-written request id.
func statusBody(b []byte, code uint32) []byte {
	b = marshalUint32(b, code)
	b = marshalString(b, "")    // message
	return marshalString(b, "") // language
}

func TestClientReadDirMalformedName(t *testing.T) {
	// body builds the SSH_FXP_NAME reply body that follows the echoed id. (The
	// 32-bit overflow of the count field is covered by TestUnmarshalCount.)
	cases := []struct {
		name    string
		body    func(b []byte) []byte
		wantErr error
	}{
		{
			name:    "count prefix truncated",
			body:    func(b []byte) []byte { return append(b, 0x00, 0x00) },
			wantErr: errShortPacket,
		},
		{
			// A huge count with no entries must fail when the loop runs out of
			// bytes, not pre-allocate or panic.
			name:    "count far exceeds entries present",
			body:    func(b []byte) []byte { return marshalUint32(b, 0x7fffffff) },
			wantErr: errShortPacket,
		},
		{
			name: "filename length exceeds bytes present",
			body: func(b []byte) []byte {
				b = marshalUint32(b, 1)             // count: one entry
				return marshalUint32(b, 0x7fffffff) // filename length, no bytes follow
			},
			wantErr: errShortPacket,
		},
		{
			name: "longname length exceeds bytes present",
			body: func(b []byte) []byte {
				b = marshalUint32(b, 1)             // count: one entry
				b = marshalString(b, "file")        // valid filename
				return marshalUint32(b, 0x7fffffff) // longname length, no bytes follow
			},
			wantErr: errShortPacket,
		},
		{
			name: "attrs truncated",
			body: func(b []byte) []byte {
				b = marshalUint32(b, 1)      // count: one entry
				b = marshalString(b, "file") // filename
				b = marshalString(b, "file") // longname
				// Attr flags claim a size field, but the 8 bytes never follow.
				return marshalUint32(b, sshFileXferAttrSize)
			},
			wantErr: errShortPacket,
		},
		{
			name: "second entry truncated mid-loop",
			body: func(b []byte) []byte {
				b = marshalUint32(b, 2)             // declare two entries
				b = dirEntry(b, "file1")            // first entry is well-formed
				return marshalUint32(b, 0x7fffffff) // second filename length, no bytes
			},
			wantErr: errShortPacket,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := startScriptedClient(t, func(typ fxp, id uint32) (fxp, []byte, bool) {
				switch typ {
				case sshFxpOpendir:
					return sshFxpHandle, marshalString(marshalUint32(nil, id), "somehandle"), true
				case sshFxpReaddir:
					return sshFxpName, tc.body(marshalUint32(nil, id)), true
				default: // the deferred close, etc.
					return 0, nil, false
				}
			})
			defer c.Close()

			if _, err := c.ReadDir("/"); !errors.Is(err, tc.wantErr) {
				t.Fatalf("ReadDir error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestClientReadDirSuccess is the positive counterpart: a well-formed listing
// (two entries, then an EOF status) must be parsed into the expected entries
// with no error, proving the hardened loop still accepts valid replies.
func TestClientReadDirSuccess(t *testing.T) {
	var readdirs int
	c := startScriptedClient(t, func(typ fxp, id uint32) (fxp, []byte, bool) {
		switch typ {
		case sshFxpOpendir:
			return sshFxpHandle, marshalString(marshalUint32(nil, id), "somehandle"), true
		case sshFxpReaddir:
			readdirs++
			if readdirs > 1 {
				// End of listing on the second readdir.
				return sshFxpStatus, statusBody(marshalUint32(nil, id), sshFxEOF), true
			}
			body := marshalUint32(nil, id)
			body = marshalUint32(body, 2) // two entries
			body = dirEntry(body, "file1")
			body = dirEntry(body, "file2")
			return sshFxpName, body, true
		case sshFxpClose:
			return sshFxpStatus, statusBody(marshalUint32(nil, id), sshFxOk), true
		default:
			return 0, nil, false
		}
	})
	defer c.Close()

	entries, err := c.ReadDir("/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Name() != "file1" || entries[1].Name() != "file2" {
		t.Errorf("entries = [%q %q], want [file1 file2]", entries[0].Name(), entries[1].Name())
	}
}

// Issue #418: panic in clientConn.recv when the sid is incomplete.
func TestClientNoSid(t *testing.T) {
	stream := new(bytes.Buffer)
	sendPacket(stream, &sshFxVersionPacket{Version: sftpProtocolVersion})
	// Next packet has the sid cut short after two bytes.
	stream.Write([]byte{0, 0, 0, 10, 0, 0})

	c, err := NewClientPipe(stream, &sink{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Stat("anything")
	if !errors.Is(err, ErrSSHFxConnectionLost) {
		t.Fatal("expected ErrSSHFxConnectionLost, got", err)
	}
}

// eofReader fails the test if it is read again after returning io.EOF.
type eofReader struct {
	*strings.Reader
	t   *testing.T
	eof bool
}

func (r *eofReader) Read(b []byte) (int, error) {
	if r.eof {
		r.t.Error("Read after io.EOF")
	}
	n, err := r.Reader.Read(b)
	r.eof = err == io.EOF
	return n, err
}

// Issue #668 in github
// ReadFrom returned nil when writing a short final chunk failed.
func TestClientReadFromWriteError(t *testing.T) {
	for _, tt := range []struct {
		name       string
		maxPacket  int
		concurrent bool
		okWrites   int
		wantErr    bool
		wantOffset int64
	}{
		{"short chunk", 8, false, 0, true, 0},
		{"short chunk after full chunk", 4, false, 1, true, 4},
		{"no error", 4, false, 2, false, 5},
		{"concurrent", 4, true, 1, true, 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0
			c := startScriptedClient(t, func(_ fxp, id uint32) (fxp, []byte, bool) {
				code := uint32(sshFxOk)
				if writes >= tt.okWrites {
					code = sshFxFailure
				}
				writes++
				return sshFxpStatus, statusBody(marshalUint32(nil, id), code), true
			}, MaxPacket(tt.maxPacket), UseConcurrentWrites(tt.concurrent))
			defer c.Close()

			f := &File{c: c, path: "somefile", handle: "somehandle"}
			n, err := f.ReadFrom(&eofReader{Reader: strings.NewReader("hello"), t: t})
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if n != 5 {
				t.Errorf("n = %d, want 5", n)
			}
			if f.offset != tt.wantOffset {
				t.Errorf("offset = %d, want %d", f.offset, tt.wantOffset)
			}
		})
	}
}

// truncatedReader fails with io.ErrUnexpectedEOF after its data, like the
// body of an HTTP response that was cut off.
type truncatedReader struct {
	*strings.Reader
}

func (r truncatedReader) Read(b []byte) (int, error) {
	n, err := r.Reader.Read(b)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

// ReadFrom returned nil when the reader failed with io.ErrUnexpectedEOF.
func TestClientReadFromUnexpectedEOF(t *testing.T) {
	for _, tt := range []struct {
		name       string
		maxPacket  int
		concurrent bool
	}{
		{"short chunk", 8, false},
		{"full chunk", 5, false},
		{"concurrent", 4, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := startScriptedClient(t, func(_ fxp, id uint32) (fxp, []byte, bool) {
				return sshFxpStatus, statusBody(marshalUint32(nil, id), sshFxOk), true
			}, MaxPacket(tt.maxPacket), UseConcurrentWrites(tt.concurrent))
			defer c.Close()

			f := &File{c: c, path: "somefile", handle: "somehandle"}
			n, err := f.ReadFrom(truncatedReader{strings.NewReader("hello")})
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("error = %v, want %v", err, io.ErrUnexpectedEOF)
			}
			if n != 5 {
				t.Errorf("n = %d, want 5", n)
			}
		})
	}
}
