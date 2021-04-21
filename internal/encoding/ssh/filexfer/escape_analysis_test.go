// +build escape_analysis

package filexfer

import (
	"bytes"
	"io"
	"io/ioutil"
	"sync/atomic"
	"testing"
)

var heapPacket = new(OpenPacket)
var heapExtAttrib = new(ExtendedAttribute)

type PacketMarshaler interface {
	MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error)
}

var requestID uint32

func testEscapeAnalysisSendPacket(w io.Writer, p PacketMarshaler) error {
	reqid := atomic.AddUint32(&requestID, 1)

	header, payload, err := p.MarshalPacket(reqid, nil)
	if err != nil {
		return err
	}

	n, err := w.Write(header)
	if err != nil {
		return err
	}
	if n < len(header) {
		return io.ErrShortWrite
	}

	if len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return err
		}
		if n < len(payload) {
			return io.ErrShortWrite
		}
	}

	return nil
}

func TestEscapeAnalysisSendPacket(t *testing.T) {
	const (
		filename = "/foo"
		perms    = 0x87654321
	)

	p := &OpenPacket{
		Filename: filename,
		PFlags:   FlagRead,
		Attrs: Attributes{
			Flags:       AttrPermissions,
			Permissions: perms,
		},
	}

	if err := testEscapeAnalysisSendPacket(ioutil.Discard, p); err != nil {
		panic(err)
	}

	var stackPacket OpenPacket
	println("escape_analysis_test.go: sending: heap:", heapPacket, "packet:", &p, "stack:", &stackPacket)
}

func TestEscapeAnalysisRecvPacket(t *testing.T) {
	const (
		id       = 42
		filename = "/foo"
		perms    = 0x87654321
	)

	data := []byte{
		0x00, 0x00, 0x00, 25,
		3,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 4, '/', 'f', 'o', 'o',
		0x00, 0x00, 0x00, 1,
		0x00, 0x00, 0x00, 0x04,
		0x87, 0x65, 0x43, 0x21,
	}

	b := make([]byte, len(data)-4) // Pick a super narrow buffer.

	var p RequestPacket
	if err := p.ReadFrom(bytes.NewReader(data), b); err != nil {
		panic(err)
	}

	if p.RequestID != id {
		panic("ReadFrom(): unexpected value")
	}

	switch req := p.Request.(type) {
	case *OpenPacket:
		if req.Filename != filename {
			panic("RequestPacket(): unexpected value")
		}

		if req.PFlags != FlagRead {
			panic("RequestPacket(): unexpected value")
		}

		if req.Attrs.Flags != AttrPermissions {
			panic("RequestPacket(): unexpected value")
		}

		if req.Attrs.Permissions != perms {
			panic("RequestPacket(): unexpected value")
		}

	default:
		panic("unexpected type")
	}

	var stackPacket OpenPacket
	println("escape_analysis_test.go: receiving: heap:", heapPacket, "packet:", &p, "stack:", &stackPacket)
}

func TestEscapeAnalysisExtendedAttribute(t *testing.T) {
	buf := NewBuffer(make([]byte, 0, DefaultMaxPacketLength))

	attr := &ExtendedAttribute{
		Type: "foo",
		Data: "bar",
	}
	attr.MarshalInto(buf)

	var stackExtAttrib ExtendedAttribute
	println("escape_analysis_test.go: ext_attrib: heap:", heapExtAttrib, "attr:", attr, "stack:", &stackExtAttrib)
}
