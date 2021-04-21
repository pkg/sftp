// Package filexfer implements the wire encoding for secsh-filexfer as described in https://tools.ietf.org/html/draft-ietf-secsh-filexfer-02
package filexfer

// Packet defines the behavior of an SFTP packet.
type Packet interface {
	// Type returns the SSH_FXP_xy value associated with the specific packet.
	Type() PacketType

	// MarshalPacket is the primary intended way to encode a packet.
	// The request-id for the packet is set from reqid.
	//
	// An optional buffer may be given in b.
	// If the buffer has a minimum capacity, it shall be truncated and used to marshal the header into.
	// The minimum capacity for the packet must be a constant expression, and should be at least 9.
	//
	// It shall return the main body of the encoded packet in header,
	// and may optionally return an additional payload to be written immediately after the header.
	//
	// It shall encode in the first 4-bytes of the header the proper length of the rest of the header+payload.
	MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error)

	// UnmarshalPacketBody decodes a packet body from the given Buffer.
	// It is assumed that the common header values of the length, type and request-id have already been consumed.
	UnmarshalPacketBody(buf *Buffer) error
}

// ComposePacket converts returns from MarshalPacket into an equivalent call to MarshalBinary.
func ComposePacket(header, payload []byte, err error) ([]byte, error) {
	return append(header, payload...), err
}

// Default length values,
// Defined in draft-ietf-secsh-filexfer-02 section 3.
const (
	DefaultMaxPacketLength = 34000
	DefaultMaxDataLength   = 32768
)
