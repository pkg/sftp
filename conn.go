package sftp

import (
	"encoding"
	"io"
	"sync"
)

// conn implements a bidirectional channel on which client and server
// connections are multiplexed

type conn struct {
	io.ReadWriteCloser
	sync.Mutex // used to serialise writes to sendPacket
}

func (c *conn) recvPacket() (uint8, []byte, error) {
	return recvPacket(c)
}

func (c *conn) sendPacket(m encoding.BinaryMarshaler) error {
	c.Lock()
	defer c.Unlock()
	return sendPacket(c, m)
}
