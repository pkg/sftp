package sshfx

import (
	"encoding/binary"
	"errors"
	"slices"
	"strconv"
	"unsafe"

	"github.com/pkg/sftp/v2/internal/sync"
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
	Err error
}

var bufPool = sync.NewPool[Buffer](64)

// NewBuffer creates and initializes a new buffer using buf as its initial contents.
// The new buffer takes ownership of buf, and the caller should not use buf after this call.
//
// In most cases, new(Buffer) (or just declaring a Buffer variable) is sufficient to initialize a buffer.
func NewBuffer(buf []byte) *Buffer {
	return &Buffer{
		b: buf,
	}
}

// NewMarshalBuffer creates a new buffer ready to start marshaling a Packet into.
// It preallocates enough space for uint32(length), and size more bytes.
func NewMarshalBuffer(size int) *Buffer {
	return NewBuffer(make([]byte, 4+size))
}

// Bytes returns a slice of length b.Len() holding the unconsumed bytes in the buffer.
// The slice is valid for use only until the next buffer modification;
// that is, only until the next call to an Append or Consume method.
func (b *Buffer) Bytes() []byte {
	return b.b[b.off:]
}

// HintReturn returns the underlying byte slice with full length, so that it can be returned to a buffer pool.
// The returned value should not be used at all until the full typed packet has been parsed out of this buffer.
func (b *Buffer) HintReturn() []byte {
	return b.b[:cap(b.b)]
}

// Len returns the number of unconsumed bytes in the buffer.
func (b *Buffer) Len() int { return len(b.b) - b.off }

// Cap returns the capacity of the buffer’s underlying byte slice;
// that is, the total space allocated for the buffer’s data.
func (b *Buffer) Cap() int { return cap(b.b) }

// Grow grows the buffer's capacity, if necessary, to guarantee space for another n bytes.
// After Grow(n), at least n bytes can be written to the buffer without another allocation.
// If n is negative, Grow will panic.
func (b *Buffer) Grow(n int) {
	b.b = slices.Grow(b.b, n)
}

// Reset resets the buffer to be empty, but it retains the underlying storage for use by future appends.
func (b *Buffer) Reset() {
	*b = Buffer{
		b: b.b[:0],
	}
}

// StartPacket resets and initializes the buffer to be ready to start marshaling a packet into.
// It truncates the buffer, reserves space for uint32(length), then appends the given packet type and request id.
func (b *Buffer) StartPacket(packetType PacketType, requestID uint32) {
	b.Reset()

	// uint32(length) + uint8(type) + uint32(request-id)
	b.Grow(4 + 1 + 4)

	b.b = b.b[:4]
	b.AppendUint8(uint8(packetType))
	b.AppendUint32(requestID)
}

// Packet finalizes the packet started from [StartPacket].
// It is expected that this will end the ownership of the underlying byte slice,
// so the returned byte slices may be reused the same as any other byte slice,
// the caller should not use this buffer after this call.
//
// It writes the packet body length into the first four bytes of the buffer in network byte order (big endian).
// The packet body length is the length of this buffer less the 4-byte length itself, plus the length of payload.
//
// It is assumed that no [Consume] methods have been called on this buffer,
// so it returns the whole underlying slice.
func (b *Buffer) Packet(payload []byte) (header, payloadPassThru []byte, err error) {
	// -uint32(length) + struct(header) + raw(payload)
	b.PutLength(-4 + len(b.b) + len(payload))

	return b.b, payload, nil
}

func (b *Buffer) checkLen(length int) bool {
	if b.Err != nil {
		return false
	}

	// The strconv.IntSize <= 32 short-circuits to false on 64-bit,
	// which elides the check for a negative length on architectures,
	// where uint32 cannot overflow to a negative int value.
	if (strconv.IntSize <= 32 && length < 0) || b.Len() < length {
		b.off = len(b.b)
		b.Err = ErrShortPacket
		return false
	}

	return true
}

// ConsumeUint8 consumes a single byte from the buffer.
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
func (b *Buffer) ConsumeUint8() uint8 {
	if !b.checkLen(1) {
		return 0
	}

	var v uint8
	v, b.off = b.b[b.off], b.off+1
	return v
}

// AppendUint8 appends a single byte into the buffer.
func (b *Buffer) AppendUint8(v uint8) {
	b.b = append(b.b, v)
}

// ConsumeBool consumes a single byte from the buffer, and returns true if that byte is non-zero.
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
func (b *Buffer) ConsumeBool() bool {
	return b.ConsumeUint8() != 0
}

// AppendBool appends a single bool into the buffer.
// It encodes it as a single byte, with false as 0, and true as 1.
func (b *Buffer) AppendBool(v bool) {
	if v {
		b.AppendUint8(1)
	} else {
		b.AppendUint8(0)
	}
}

// ConsumeUint16 consumes a single uint16 from the buffer, in network byte order (big-endian).
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
func (b *Buffer) ConsumeUint16() uint16 {
	if !b.checkLen(2) {
		return 0
	}

	v := binary.BigEndian.Uint16(b.b[b.off:])
	b.off += 2
	return v
}

// AppendUint16 appends single uint16 into the buffer, in network byte order (big-endian).
func (b *Buffer) AppendUint16(v uint16) {
	b.b = append(b.b,
		byte(v>>8),
		byte(v>>0),
	)
}

// unmarshalPacketLength is used internally to read the packet length.
// It is unsafe, and so not exported.
// Its use should be avoided even within this package.
func unmarshalPacketLength(b []byte) uint32 {
	return binary.BigEndian.Uint32(b[:4])
}

// ConsumeUint32 consumes a single uint32 from the buffer, in network byte order (big-endian).
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
func (b *Buffer) ConsumeUint32() uint32 {
	if !b.checkLen(4) {
		return 0
	}

	v := binary.BigEndian.Uint32(b.b[b.off:])
	b.off += 4
	return v
}

// AppendUint32 appends a single uint32 into the buffer, in network byte order (big-endian).
func (b *Buffer) AppendUint32(v uint32) {
	b.b = append(b.b,
		byte(v>>24),
		byte(v>>16),
		byte(v>>8),
		byte(v>>0),
	)
}

// ConsumeCount consumes a single uint32 count from the buffer, in network byte order (big-endian) as an int.
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
func (b *Buffer) ConsumeCount() (int, error) {
	count := int(b.ConsumeUint32())
	return count, b.Err
}

// AppendCount appends a single int length as a uint32 into the buffer, in network byte order (big-endian).
func (b *Buffer) AppendCount(v int) {
	b.AppendUint32(uint32(v))
}

// ConsumeUint64 consumes a single uint64 from the buffer, in network byte order (big-endian).
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
func (b *Buffer) ConsumeUint64() uint64 {
	if !b.checkLen(8) {
		return 0
	}

	v := binary.BigEndian.Uint64(b.b[b.off:])
	b.off += 8
	return v
}

// AppendUint64 appends a single uint64 into the buffer, in network byte order (big-endian).
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

// ConsumeInt64 consumes a single int64 from the buffer, in network byte order (big-endian) with two’s complement.
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
func (b *Buffer) ConsumeInt64() int64 {
	return int64(b.ConsumeUint64())
}

// AppendInt64 appends a single int64 into the buffer, in network byte order (big-endian) with two’s complement.
func (b *Buffer) AppendInt64(v int64) {
	b.AppendUint64(uint64(v))
}

// ConsumeBytes consumes a single string of raw binary data from the buffer.
// A string is a uint32 length, followed by that number of raw bytes.
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
//
// The returned slice aliases the buffer contents, and is valid only as long as the buffer is not reused;
// that is, only until the next call to [Reset], [PutLength], [StartPacket], or [UnmarshalBinary].
//
// In no case will consuming calls return overlapping slice aliases,
// and append calls are guaranteed to not disturb this slice alias.
func (b *Buffer) ConsumeBytes() []byte {
	length := int(b.ConsumeUint32())

	if length == 0 {
		// Short-circuit empty strings.
		return nil
	}

	if !b.checkLen(length) {
		return nil
	}

	v := b.b[b.off:]
	if len(v) > length || cap(v) > length {
		v = slices.Clip(v[:length])
	}
	b.off += length
	return v
}

// ConsumeBytesCopy consumes and returns a copy of a single string of raw binary data from the buffer.
// A string is a uint32 length, followed by that number of raw bytes.
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
//
// The returned slice does not alias any buffer contents,
// and will therefore be valid even if the buffer is later reused.
//
// If hint has sufficient capacity to hold the data, it will be reused and overwritten,
// otherwise a new backing slice will be allocated and returned.
func (b *Buffer) ConsumeBytesCopy(hint []byte) []byte {
	data := b.ConsumeBytes()

	if len(data) == 0 {
		// Short-circuit empty strings to the zero-length slice of the hint.
		// Nota bene: if hint == nil, then this will return the nil slice.
		return hint[:0]
	}

	return append(hint[:0], data...)
}

// AppendBytes appends a single string of raw binary data into the buffer.
// A string is a uint32 length, followed by that number of raw bytes.
func (b *Buffer) AppendBytes(v []byte) {
	// uint32(length) + raw(data)
	b.Grow(4 + len(v)) // ensure at most one allocation

	b.AppendUint32(uint32(len(v)))
	b.b = append(b.b, v...)
}

// ConsumeString consumes a single string of binary data as a Go string from the buffer.
// A string is a uint32 length, followed by that number of raw bytes.
// If the buffer does not have enough data, it will set Err to ErrShortPacket.
//
// NOTE: Go implicitly assumes that strings contain UTF-8 encoded data.
// All caveats on using arbitrary binary data in Go strings applies.
func (b *Buffer) ConsumeString() string {
	return string(b.ConsumeBytes())
}

// AppendString appends a single string of binary data into the buffer.
// A string is a uint32 length, followed by that number of raw bytes.
func (b *Buffer) AppendString(v string) {
	// This uses an unsafe bytes slice reference of v to avoid a potential copy to a mutable byte slice.
	// Fortunately, AppendBytes treats the bytes slice as immutable, so this should not crash.
	b.AppendBytes(unsafe.Slice(unsafe.StringData(v), len(v)))
}

// PutLength writes the given size into the first four bytes of the buffer in network byte order (big endian).
func (b *Buffer) PutLength(size int) {
	if grow := 4 - len(b.b); grow > 0 {
		b.b = append(b.b, make([]byte, grow)...)
	}

	binary.BigEndian.PutUint32(b.b, uint32(size))
}

// MarshalSize returns the number of bytes that the packet would marshal into.
// This excludes the uint32(length).
func (b *Buffer) MarshalSize() int {
	// raw(data)
	return len(b.b)
}

// MarshalBinary returns a clone of the full internal buffer.
func (b *Buffer) MarshalBinary() ([]byte, error) {
	return slices.Clone(b.b), nil
}

// UnmarshalBinary sets the internal buffer of b to be a clone of data, and zeros the internal offset.
func (b *Buffer) UnmarshalBinary(data []byte) error {
	b.b = append(b.b[:0], data...)
	b.off = 0
	return nil
}
