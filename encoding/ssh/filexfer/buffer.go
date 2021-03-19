package filexfer

import (
	"encoding/binary"
	"errors"
)

// Various encoding errors.
var (
	ErrShortPacket = errors.New("packet too short")
)

// Buffer wraps up the various encoding details of the SSH format.
//
// Data types are encoded as per section 4 from https://tools.ietf.org/html/draft-ietf-secsh-architecture-09#page-8
type Buffer struct {
	b []byte
}

// NewBuffer creates and initializes a new Buffer using buf as its initial contents.
func NewBuffer(b []byte) *Buffer {
	return &Buffer{
		b: b,
	}
}

// NewMarshalBuffer creates an initializes a new Buffer ready to start marshaling a Packet into.
// It preallocates enough space for size additional bytes of data above the 4-byte length.
func NewMarshalBuffer(size int) *Buffer {
	return NewBuffer(make([]byte, 4, 4+size))
}

// Bytes returns a slice of length b.Len() holding the unconsumed bytes in the Buffer.
func (b *Buffer) Bytes() []byte {
	return b.b
}

// Len returns the number of unconsumed bytes in the Buffer.
func (b *Buffer) Len() int {
	return len(b.b)
}

// ConsumeBool consumes a single byte from the Buffer, and returns true if that byte is non-zero.
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeBool() (bool, error) {
	if len(b.b) < 1 {
		return false, ErrShortPacket
	}

	var v uint8
	v, b.b = b.b[0], b.b[1:]
	return v != 0, nil
}

// AppendBool appends a single bool into the Buffer.
// It encodes it as a single byte, with false as 0, and true as 1.
func (b *Buffer) AppendBool(v bool) {
	if v {
		b.b = append(b.b, 1)
	} else {
		b.b = append(b.b, 0)
	}
}

// ConsumeUint8 consumes a single byte from the Buffer.
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeUint8() (uint8, error) {
	if len(b.b) < 1 {
		return 0, ErrShortPacket
	}

	var v uint8
	v, b.b = b.b[0], b.b[1:]
	return v, nil
}

// AppendUint8 appends a single byte into the Buffer.
func (b *Buffer) AppendUint8(v uint8) {
	b.b = append(b.b, v)
}

// ConsumeUint16 consumes a single uint16 from the Buffer, in network byte order (big-endian).
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeUint16() (uint16, error) {
	if len(b.b) < 2 {
		return 0, ErrShortPacket
	}

	var v uint16
	v, b.b = uint16(b.b[0])<<8|uint16(b.b[1]), b.b[2:]
	return v, nil
}

// AppendUint16 appends single uint16 into the Buffer, in network byte order (big-endian).
func (b *Buffer) AppendUint16(v uint16) {
	b.b = append(b.b,
		byte(v>>8),
		byte(v>>0),
	)
}

// unmarshalUint32 is used internally to read the packet length.
// It is unsafe, and so not exported.
// Even avoid using it in this package.
func unmarshalUint32(b []byte) (uint32, []byte) {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), b[4:]
}

// ConsumeUint32 consumes a single uint32 from the Buffer, in network byte order (big-endian).
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeUint32() (uint32, error) {
	if len(b.b) < 4 {
		return 0, ErrShortPacket
	}

	var v uint32
	v, b.b = unmarshalUint32(b.b)
	return v, nil
}

// AppendUint32 appends a single uint32 into the Buffer, in network byte order (big-endian).
func (b *Buffer) AppendUint32(v uint32) {
	b.b = append(b.b,
		byte(v>>24),
		byte(v>>16),
		byte(v>>8),
		byte(v>>0),
	)
}

// ConsumeUint64 consumes a single uint64 from the Buffer, in network byte order (big-endian).
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeUint64() (uint64, error) {
	if len(b.b) < 8 {
		return 0, ErrShortPacket
	}

	var h, l uint32
	h, b.b = unmarshalUint32(b.b)
	l, b.b = unmarshalUint32(b.b)
	return uint64(h)<<32 | uint64(l), nil
}

// AppendUint64 appends a single uint64 into the Buffer, in network byte order (big-endian).
func (b *Buffer) AppendUint64(v uint64) {
	b.b = append(b.b,
		byte(v>>56),
		byte(v>>48),
		byte(v>>40),
		byte(v>>32),
		byte(v>>24),
		byte(v>>16),
		byte(v>>8),
		byte(v>>0),
	)
}

// ConsumeInt64 consumes a single int64 from the Buffer, in network byte order (big-endian) with two’s complement.
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeInt64() (int64, error) {
	u, err := b.ConsumeUint64()
	if err != nil {
		return 0, err
	}

	return int64(u), err
}

// AppendInt64 appends a single uint64 into the Buffer, in network byte order (big-endian) with two’s complement.
func (b *Buffer) AppendInt64(v uint64) {
	b.AppendUint64(uint64(v))
}

// ConsumeByteSlice consumes a single string of raw binary data from the Buffer.
// A string is a uint32 length, followed by that number of raw bytes.
// If Buffer does not have enough data, or defines a length larger than available, it will return ErrShortPacket.
func (b *Buffer) ConsumeByteSlice() ([]byte, error) {
	length, err := b.ConsumeUint32()
	if err != nil {
		return nil, err
	}

	if b.Len() < int(length) {
		return nil, ErrShortPacket
	}

	var v []byte
	v, b.b = b.b[:length], b.b[length:]
	return v, nil
}

// AppendByteSlice appends a single string of raw binary data into the Buffer.
// A string is a uint32 length, followed by that number of raw bytes.
func (b *Buffer) AppendByteSlice(v []byte) {
	b.AppendUint32(uint32(len(v)))
	b.b = append(b.b, v...)
}

// ConsumeString consumes a single string of UTF-8 encoded text from the Buffer.
// A string is a uint32 length, followed by that number of raw bytes.
// If Buffer does not have enough data, or defines a length larger than available, it will return ErrShortPacket.
func (b *Buffer) ConsumeString() (string, error) {
	v, err := b.ConsumeByteSlice()
	if err != nil {
		return "", err
	}

	return string(v), nil
}

// AppendString appends a single string of UTF-8 encoded text into the Buffer.
// A string is a uint32 length, followed by that number of raw bytes.
func (b *Buffer) AppendString(v string) {
	b.AppendByteSlice([]byte(v))
}

// PutLength writes the given size into the first four bytes of the Buffer in network byte order (big endian).
func (b *Buffer) PutLength(size int) {
	if len(b.b) < 4 {
		b.b = append(b.b, make([]byte, 4-len(b.b))...)
	}

	binary.BigEndian.PutUint32(b.b, uint32(size))
}
