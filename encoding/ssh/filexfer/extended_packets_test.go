package filexfer

import (
	"bytes"
	"testing"
)

type testExtendedData struct {
	value uint8
}

func (d *testExtendedData) MarshalBinary() ([]byte, error) {
	buf := NewBuffer(make([]byte, 0, 4))

	buf.AppendUint8(d.value ^ 0x2a)

	return buf.Bytes(), nil
}

func (d *testExtendedData) UnmarshalBinary(data []byte) error {
	buf := NewBuffer(data)

	v, err := buf.ConsumeUint8()
	if err != nil {
		return err
	}

	d.value = v ^ 0x2a

	return nil
}

func TestExtendedPacketNoData(t *testing.T) {
	const (
		id              = 42
		extendedRequest = "foo@example"
	)

	p := &ExtendedPacket{
		RequestID:       id,
		ExtendedRequest: extendedRequest,
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 20,
		200,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 11, 'f', 'o', 'o', '@', 'e', 'x', 'a', 'm', 'p', 'l', 'e',
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = ExtendedPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.ExtendedRequest != extendedRequest {
		t.Errorf("UnmarshalBinary(): ExtendedRequest was %q, but expected %q", p.ExtendedRequest, extendedRequest)
	}
}

func TestExtendedPacketTestData(t *testing.T) {
	const (
		id              = 42
		extendedRequest = "foo@example"
		textValue       = 13
	)

	const value = 13

	p := &ExtendedPacket{
		RequestID:       id,
		ExtendedRequest: extendedRequest,
		Data: &testExtendedData{
			value: textValue,
		},
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 21,
		200,
		0x00, 0x00, 0x00, 42,
		0x00, 0x00, 0x00, 11, 'f', 'o', 'o', '@', 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x27,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = ExtendedPacket{
		Data: new(testExtendedData),
	}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.ExtendedRequest != extendedRequest {
		t.Errorf("UnmarshalBinary(): ExtendedRequest was %q, but expected %q", p.ExtendedRequest, extendedRequest)
	}

	if data, ok := p.Data.(*testExtendedData); !ok {
		t.Errorf("UnmarshalBinary(): Data was type %T, but expected %T", p.Data, data)

	} else if data.value != value {
		t.Errorf("UnmarshalBinary(): Data.value was %#x, but expected %#x", data.value, value)
	}

	*p = ExtendedPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if p.ExtendedRequest != extendedRequest {
		t.Errorf("UnmarshalBinary(): ExtendedRequest was %q, but expected %q", p.ExtendedRequest, extendedRequest)
	}

	wantBuffer := []byte{0x27}

	if data, ok := p.Data.(*Buffer); !ok {
		t.Errorf("UnmarshalBinary(): Data was type %T, but expected %T", p.Data, data)

	} else if !bytes.Equal(data.b, wantBuffer) {
		t.Errorf("UnmarshalBinary(): Data was %X, but expected %X", data.b, wantBuffer)
	}
}

func TestExtendedReplyNoData(t *testing.T) {
	const (
		id = 42
	)

	p := &ExtendedReplyPacket{
		RequestID: id,
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 5,
		201,
		0x00, 0x00, 0x00, 42,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = ExtendedReplyPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}
}

func TestExtendedReplyPacketTestData(t *testing.T) {
	const (
		id        = 42
		textValue = 13
	)

	const value = 13

	p := &ExtendedReplyPacket{
		RequestID: id,
		Data: &testExtendedData{
			value: textValue,
		},
	}

	data, err := p.MarshalBinary()
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := []byte{
		0x00, 0x00, 0x00, 6,
		201,
		0x00, 0x00, 0x00, 42,
		0x27,
	}

	if !bytes.Equal(data, want) {
		t.Fatalf("Marshal() = %X, but wanted %X", data, want)
	}

	*p = ExtendedReplyPacket{
		Data: new(testExtendedData),
	}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	if data, ok := p.Data.(*testExtendedData); !ok {
		t.Errorf("UnmarshalBinary(): Data was type %T, but expected %T", p.Data, data)

	} else if data.value != value {
		t.Errorf("UnmarshalBinary(): Data.value was %#x, but expected %#x", data.value, value)
	}

	*p = ExtendedReplyPacket{}

	// UnmarshalBinary assumes the uint32(length) + uint8(type) have already been consumed.
	if err := p.UnmarshalBinary(data[5:]); err != nil {
		t.Fatal("unexpected error:", err)
	}

	if p.RequestID != uint32(id) {
		t.Errorf("UnmarshalBinary(): RequestID was %d, but expected %d", p.RequestID, id)
	}

	wantBuffer := []byte{0x27}

	if data, ok := p.Data.(*Buffer); !ok {
		t.Errorf("UnmarshalBinary(): Data was type %T, but expected %T", p.Data, data)

	} else if !bytes.Equal(data.b, wantBuffer) {
		t.Errorf("UnmarshalBinary(): Data was %X, but expected %X", data.b, wantBuffer)
	}
}
