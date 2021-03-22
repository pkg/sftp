package filexfer

import (
	"encoding/binary"
	"errors"
)

// Various encoding errors.
var (
	ErrShortPacket = errors.New("packet too short")
	ErrLongPacket  = errors.New("packet too long")
)

// Buffer wraps up the various encoding details of the SSH format.
//
// Data types are encoded as per section 4 from https://tools.ietf.org/html/draft-ietf-secsh-architecture-09#page-8
type Buffer struct {
	b   []byte
	off int
}

// NewBuffer creates and initializes a new Buffer using buf as its initial contents.
// The new Buffer takes ownership of buf, and the caller should not use buf after this call.
//
// In most cases, new(Buffer) (or just declaring a Buffer variable) is sufficient to initialize a Buffer.
func NewBuffer(buf []byte) *Buffer {
	return &Buffer{
		b: buf,
	}
}

// NewMarshalBuffer creates an initializes a new Buffer ready to start marshaling a Packet into.
// It prepopulates 4 bytes for length, the 1-byte packetType, and the 4-byte requestID.
// It preallocates enough space for an additional size bytes of data above the prepopulated values.
func NewMarshalBuffer(packetType PacketType, requestID uint32, size int) *Buffer {
	buf := NewBuffer(make([]byte, 4, 4+1+4+size))

	buf.AppendUint8(uint8(packetType))
	buf.AppendUint32(requestID)

	return buf
}

// Bytes returns a slice of length b.Len() holding the unconsumed bytes in the Buffer.
// The slice is valid for use only until the next buffer modification
// (that is, only until the next call to an Append or Consume method).
func (b *Buffer) Bytes() []byte {
	return b.b[b.off:]
}

// Packet finalizes the packet started from NewMarshalPacket.
// It is expected that this will end the ownership of the underlying byte-slice,
// and so the returned byte-slices may potentially be returned to a memory pool the same as any other byte-slice.
// The caller should not use this Buffer at all after this call.
//
// It writes the packet body length into the first four bytes of the Buffer in network byte order (big endian).
// The packet body length is the size of the Buffer less the 4-byte length itself, plus the length of payload.
//
// It is assumed that no Consume methods have been called on this Buffer,
// and so it returns the whole underlying slice.
func (b *Buffer) Packet(payload []byte) (header, payloadPassThru []byte, err error) {
	b.PutLength(len(b.b) - 4 + len(payload))

	return b.b, payload, nil
}

// Len returns the number of unconsumed bytes in the Buffer.
func (b *Buffer) Len() int {
	return len(b.b) - b.off
}

// ConsumeUint8 consumes a single byte from the Buffer.
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeUint8() (uint8, error) {
	if b.Len() < 1 {
		return 0, ErrShortPacket
	}

	var v uint8
	v, b.off = b.b[b.off], b.off+1
	return v, nil
}

// AppendUint8 appends a single byte into the Buffer.
func (b *Buffer) AppendUint8(v uint8) {
	b.b = append(b.b, v)
}

// ConsumeBool consumes a single byte from the Buffer, and returns true if that byte is non-zero.
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeBool() (bool, error) {
	v, err := b.ConsumeUint8()
	if err != nil {
		return false, err
	}

	return v != 0, nil
}

// AppendBool appends a single bool into the Buffer.
// It encodes it as a single byte, with false as 0, and true as 1.
func (b *Buffer) AppendBool(v bool) {
	if v {
		b.AppendUint8(1)
	} else {
		b.AppendUint8(0)
	}
}

// ConsumeUint16 consumes a single uint16 from the Buffer, in network byte order (big-endian).
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeUint16() (uint16, error) {
	if b.Len() < 2 {
		return 0, ErrShortPacket
	}

	v := binary.BigEndian.Uint16(b.b[b.off:])
	b.off += 2
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
// Even in this package, its use should be avoided.
func unmarshalUint32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}

// ConsumeUint32 consumes a single uint32 from the Buffer, in network byte order (big-endian).
// If Buffer does not have enough data, it will return ErrShortPacket.
func (b *Buffer) ConsumeUint32() (uint32, error) {
	if b.Len() < 4 {
		return 0, ErrShortPacket
	}

	v := binary.BigEndian.Uint32(b.b[b.off:])
	b.off += 4
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
	if b.Len() < 8 {
		return 0, ErrShortPacket
	}

	v := binary.BigEndian.Uint64(b.b[b.off:])
	b.off += 8
	return v, nil
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

	v := b.b[b.off:]
	if len(v) > int(length) {
		v = v[:length:length]
	}
	b.off += int(length)
	return v, nil
}

// AppendByteSlice appends a single string of raw binary data into the Buffer.
// A string is a uint32 length, followed by that number of raw bytes.
func (b *Buffer) AppendByteSlice(v []byte) {
	b.AppendUint32(uint32(len(v)))
	b.b = append(b.b, v...)
}

// ConsumeString consumes a single string of binary data from the Buffer.
// A string is a uint32 length, followed by that number of raw bytes.
// If Buffer does not have enough data, or defines a length larger than available, it will return ErrShortPacket.
//
// NOTE: Go implicitly assumes that strings contain UTF-8 encoded data.
// All caveats on using arbitrary binary data in Go strings applies.
func (b *Buffer) ConsumeString() (string, error) {
	v, err := b.ConsumeByteSlice()
	if err != nil {
		return "", err
	}

	return string(v), nil
}

// AppendString appends a single string of binary data into the Buffer.
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

// MarshalBinary returns the remaining binary data in the Buffer as a byte slice.
// This aliases the internal buffer, and so comes with the same caveats as Bytes().
//
// This function is a thin wrapper of Bytes() solely to implement encoding.BinaryMarshaler.
func (b *Buffer) MarshalBinary() ([]byte, error) {
	return b.Bytes(), nil
}

// UnmarshalBinary sets the internal buffer of b to be data, and zeros any internal offset.
// To avoid additional allocations,
// UnmarshalBinary takes ownership of buf, and the caller should not use buf after this call.
func (b *Buffer) UnmarshalBinary(data []byte) error {
	b.b = data
	b.off = 0
	return nil
}
