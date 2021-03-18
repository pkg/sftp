package filexfer

// Packet defines the behavior of an SFTP packet.
type Packet interface {
	MarshalPacket() (header, payload []byte, err error)
	UnmarshalPacketBody(buf *Buffer) error
}

// ComposePacket converts returns from MarshalPacket into the returns expected by MarshalBinary.
func ComposePacket(header, payload []byte, err error) ([]byte, error) {
	return append(header, payload...), err
}
