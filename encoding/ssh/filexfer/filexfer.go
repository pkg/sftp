package filexfer

// Packet defines the behavior of an SFTP packet.
type Packet interface {
	MarshalPacket(reqid uint32) (header, payload []byte, err error)
	UnmarshalPacketBody(buf *Buffer) error
}

// ComposePacket converts returns from MarshalPacket into an equivalent call to MarshalBinary.
func ComposePacket(header, payload []byte, err error) ([]byte, error) {
	return append(header, payload...), err
}
