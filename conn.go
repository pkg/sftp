package sftp

import (
	"encoding"
	"io"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

// conn implements a bidirectional channel on which client and server
// connections are multiplexed.
type conn struct {
	io.Reader

	// this is the same allocator used in packet manager
	alloc *allocator

	sync.Mutex // used to serialise writes to sendPacket
	io.WriteCloser
}

// the orderID is used in server mode if the allocator is enabled.
// For the client mode just pass 0
func (c *conn) recvPacket(orderID uint32) (uint8, []byte, error) {
	return recvPacket(c, c.alloc, orderID)
}

func (c *conn) sendPacket(m encoding.BinaryMarshaler) error {
	c.Lock()
	defer c.Unlock()

	return sendPacket(c.WriteCloser, m)
}

func (c *conn) sendFXPacket(id uint32, p filexfer.Packet) error {
	header, payload, err := p.MarshalPacket(id, nil)
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

	return c.WriteCloser.Close()
}

type clientConn struct {
	conn
	wg sync.WaitGroup

	nextid uint32

	sync.Mutex                          // protects inflight
	inflight   map[uint32]chan<- result // outstanding requests

	closed chan struct{}
	err    error
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
	typ  byte
	sid  uint32
	data []byte
	err  error
}

// recv continuously reads from the server and forwards responses to the
// appropriate channel.
func (c *clientConn) recv() error {
	defer c.conn.Close()

	for {
		typ, data, err := c.recvPacket(0)
		if err != nil {
			return err
		}

		sid, _, err := unmarshalUint32Safe(data)
		if err != nil {
			return err
		}

		ch, ok := c.getChannel(sid)
		if !ok {
			// This is an unexpected occurrence. Send the error
			// back to all listeners so that they terminate
			// gracefully.
			return errors.Errorf("sid not found: %d", sid)
		}

		ch <- result{typ: typ, sid: sid, data: data}
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

func (c *clientConn) sendPacket(ch chan result, p idmarshaler) (byte, []byte, error) {
	if cap(ch) < 1 {
		ch = make(chan result, 1)
	}

	c.dispatchRequest(ch, p)
	s := <-ch
	return s.typ, s.data, s.err
}

func (c *clientConn) sendFXPacket(req, resp filexfer.Packet) error {
	id := c.nextID()
	ch := make(chan result, 1)

	c.dispatchFXPacket(ch, id, req)

	s := <-ch
	if s.err != nil {
		return s.err
	}

	if s.sid != id {
		return &unexpectedIDErr{
			want: id,
			got:  s.sid,
		}
	}

	body := filexfer.NewBuffer(s.data[4:]) // skip the ID.

	if filexfer.PacketType(s.typ) == filexfer.PacketTypeStatus {
		var status filexfer.StatusPacket

		if err := status.UnmarshalPacketBody(body); err != nil {
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
			want: uint8(filexfer.PacketTypeStatus),
			got:  s.typ,
		}
	}

	if filexfer.PacketType(s.typ) != resp.Type() {
		return &unexpectedPacketErr{
			want: uint8(resp.Type()),
			got:  s.typ,
		}
	}

	return resp.UnmarshalPacketBody(body)
}

func (c *clientConn) dispatchFXPacket(ch chan<- result, id uint32, req filexfer.Packet) {
	if !c.putChannel(ch, id) {
		// already closed.
		return
	}

	if err := c.conn.sendFXPacket(id, req); err != nil {
		if ch, ok := c.getChannel(id); ok {
			ch <- result{err: err}
		}
	}
}

// dispatchRequest should ideally only be called by race-detection tests outside of this file,
// where you have to ensure two packets are in flight sequentially after each other.
func (c *clientConn) dispatchRequest(ch chan<- result, p idmarshaler) {
	sid := p.id()

	if !c.putChannel(ch, sid) {
		// already closed.
		return
	}

	if err := c.conn.sendPacket(p); err != nil {
		if ch, ok := c.getChannel(sid); ok {
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
	return s.sendPacket(statusFromError(id, err))
}
