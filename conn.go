package sftp

import (
	"encoding"
	"io"
	"sync"

	"github.com/pkg/errors"
)

// conn implements a bidirectional channel on which client and server
// connections are multiplexed.
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

type clientConn struct {
	conn
	sync.Mutex                          // protects inflight
	inflight   map[uint32]chan<- result // outstanding requests

}

func (c *clientConn) loop(wg *sync.WaitGroup) {
	defer wg.Done()
	err := c.recv()
	if err != nil {
		c.broadcastErr(err)
	}
}

// recv continuously reads from the server and forwards responses to the
// appropriate channel.
func (c *clientConn) recv() error {
	for {
		typ, data, err := c.recvPacket()
		if err != nil {
			return err
		}
		sid, _ := unmarshalUint32(data)
		c.Lock()
		ch, ok := c.inflight[sid]
		delete(c.inflight, sid)
		c.Unlock()
		if !ok {
			// This is an unexpected occurrence. Send the error
			// back to all listeners so that they terminate
			// gracefully.
			return errors.Errorf("sid: %v not fond", sid)
		}
		ch <- result{typ: typ, data: data}
	}
}

func (c *clientConn) dispatchRequest(ch chan<- result, p idmarshaler) {
	c.Lock()
	c.inflight[p.id()] = ch
	if err := c.sendPacket(p); err != nil {
		delete(c.inflight, p.id())
		ch <- result{err: err}
	}
	c.Unlock()
}

// broadcastErr sends an error to all goroutines waiting for a response.
func (c *clientConn) broadcastErr(err error) {
	c.Lock()
	listeners := make([]chan<- result, 0, len(c.inflight))
	for _, ch := range c.inflight {
		listeners = append(listeners, ch)
	}
	c.Unlock()
	for _, ch := range listeners {
		ch <- result{err: err}
	}
}
