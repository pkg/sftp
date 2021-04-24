package sftp

import (
	"encoding"
	"io"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

// conn implements a bidirectional channel on which client and server
// connections are multiplexed.
type conn struct {
	io.Reader

	// this is the same allocator used in packet manager
	alloc *allocator

	sync.Mutex // used to serialise writes, and closes.
	io.Writer
	io.Closer
}

// the orderID is used in server mode if the allocator is enabled.
// For the client mode just pass 0
func (c *conn) recvPacket(orderID uint32) (uint8, []byte, error) {
	return recvPacket(c, c.alloc, orderID)
}

func (c *conn) writeBinary(m encoding.BinaryMarshaler) error {
	c.Lock()
	defer c.Unlock()

	return sendPacket(c.Writer, m)
}

func (c *conn) writePacket(id uint32, p sshfx.PacketMarshaller, b []byte) error {
	header, payload, err := p.MarshalPacket(id, b)
	if err != nil {
		return errors.WithStack(err)
	}

	c.Lock()
	defer c.Unlock()

	if _, err := c.Write(header); err != nil {
		return errors.WithStack(err)
	}

	if len(payload) > 0 {
		if _, err := c.Write(payload); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func (c *conn) Close() error {
	c.Lock()
	defer c.Unlock()

	return c.Closer.Close()
}

type clientConn struct {
	*conn
	wg sync.WaitGroup

	nextid  uint32
	resPool resChanPool
	bufPool *bufPool

	sync.Mutex                          // protects inflight
	inflight   map[uint32]chan<- result // outstanding requests

	closed chan struct{}
	err    error
}

func newClientConn(rd io.Reader, wr io.WriteCloser) *clientConn {
	return &clientConn{
		conn: &conn{
			Reader: rd,
			Writer: wr,
			Closer: wr,
		},
		inflight: make(map[uint32]chan<- result),
		closed:   make(chan struct{}),
	}
}

// returns the next value of c.nextid
func (c *clientConn) nextID() uint32 {
	return atomic.AddUint32(&c.nextid, 1)
}

// Wait blocks until the conn has shut down, and return the error
// causing the shutdown. It can be called concurrently from multiple
// goroutines.
func (c *clientConn) Wait() error {
	<-c.closed
	return c.err
}

// Close closes the SFTP session.
func (c *clientConn) Close() error {
	defer c.wg.Wait()
	return c.conn.Close()
}

func (c *clientConn) loop() {
	defer c.wg.Done()
	err := c.recv()
	if err != nil {
		c.broadcastErr(err)
	}
}

// result captures the result of receiving the a packet from the server
type result struct {
	pkt sshfx.RawPacket
	buf []byte // return it after you’re done with it.
	err error
}

// recv continuously reads from the server and forwards responses to the
// appropriate channel.
func (c *clientConn) recv() error {
	defer c.conn.Close()

	for {
		var pkt sshfx.RawPacket
		buf := c.bufPool.Get()

		if err := pkt.ReadFrom(c.conn.Reader, buf, 64*1024); err != nil {
			return err
		}

		ch, ok := c.getChannel(pkt.RequestID)
		if !ok {
			// This is an unexpected occurrence. Send the error
			// back to all listeners so that they terminate
			// gracefully.
			return errors.Errorf("sid not found: %d", pkt.RequestID)
		}

		ch <- result{pkt: pkt, buf: buf}
	}
}

func (c *clientConn) putChannel(ch chan<- result, sid uint32) bool {
	c.Lock()
	defer c.Unlock()

	select {
	case <-c.closed:
		// already closed with broadcastErr, return error on chan.
		ch <- result{err: ErrSSHFxConnectionLost}
		return false
	default:
	}

	c.inflight[sid] = ch
	return true
}

func (c *clientConn) getChannel(sid uint32) (chan<- result, bool) {
	c.Lock()
	defer c.Unlock()

	ch, ok := c.inflight[sid]
	delete(c.inflight, sid)

	return ch, ok
}

type idmarshaler interface {
	id() uint32
	encoding.BinaryMarshaler
}

func (c *clientConn) sendPacket(req sshfx.PacketMarshaller, resp sshfx.Packet) error {
	id := c.nextID()

	ch := c.resPool.Get()
	defer c.resPool.Put(ch)

	c.dispatchPacket(ch, id, req)

	r := <-ch
	if r.err != nil {
		// sendPacket should never return an io.EOF except through a StatusError.
		if errors.Is(r.err, io.EOF) {
			return ErrSSHFxConnectionLost
		}

		return r.err
	}

	// Because DataPacket shall not alias r.pkt.Buffer,
	// we are safe to return this buffer to the pool in all cases.
	defer c.bufPool.Put(r.buf)

	if r.pkt.RequestID != id {
		return &unexpectedIDErr{
			want: id,
			got:  r.pkt.RequestID,
		}
	}

	if r.pkt.PacketType == sshfx.PacketTypeStatus {
		var status sshfx.StatusPacket

		if err := status.UnmarshalPacketBody(&r.pkt.Data); err != nil {
			return err
		}

		return &StatusError{
			Code: uint32(status.StatusCode),
			msg:  status.ErrorMessage,
			lang: status.LanguageTag,
		}
	}

	if resp == nil {
		return &unexpectedPacketErr{
			want: uint8(sshfx.PacketTypeStatus),
			got:  uint8(r.pkt.PacketType),
		}
	}

	if r.pkt.PacketType != resp.Type() {
		return &unexpectedPacketErr{
			want: uint8(resp.Type()),
			got:  uint8(r.pkt.PacketType),
		}
	}

	return resp.UnmarshalPacketBody(&r.pkt.Data)
}

func (c *clientConn) dispatchPacket(ch chan<- result, id uint32, req sshfx.PacketMarshaller) {
	if !c.putChannel(ch, id) {
		// already closed.
		return
	}

	buf := c.bufPool.Get()
	defer c.bufPool.Put(buf)

	if err := c.conn.writePacket(id, req, buf); err != nil {
		if ch, ok := c.getChannel(id); ok {
			ch <- result{err: err}
		}
	}
}

// broadcastErr sends an error to all goroutines waiting for a response.
func (c *clientConn) broadcastErr(err error) {
	c.Lock()
	defer c.Unlock()

	bcastRes := result{err: ErrSSHFxConnectionLost}
	for sid, ch := range c.inflight {
		ch <- bcastRes

		// Replace the chan in inflight,
		// we have hijacked this chan,
		// and this guarantees always-only-once sending.
		c.inflight[sid] = make(chan<- result, 1)
	}

	c.err = err
	close(c.closed)
}

type serverConn struct {
	conn
}

func (s *serverConn) sendError(id uint32, err error) error {
	return s.writeBinary(statusFromError(id, err))
}
