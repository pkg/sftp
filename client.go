package sftp

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"math"
	"os"
	"path"
	"slices"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
	"github.com/pkg/sftp/v2/internal/sync"

	"golang.org/x/crypto/ssh"
)

type result struct {
	pkt *sshfx.RawPacket
	err error
}

type clientConn struct {
	reqid atomic.Uint32
	rd    io.Reader

	resPool *sync.WorkPool[result]

	bufPool *sync.SlicePool[[]byte, byte]
	pktPool *sync.Pool[sshfx.RawPacket]

	mu       sync.Mutex
	closed   chan struct{}
	inflight map[uint32]chan<- result
	wr       io.WriteCloser
	err      error
}

func (c *clientConn) handshake(ctx context.Context, maxPacket uint32) (map[string]string, error) {
	initPkt := &sshfx.InitPacket{
		Version: sftpProtocolVersion,
	}

	data, err := initPkt.MarshalBinary()
	if err != nil {
		return nil, err
	}

	if _, err := c.wr.Write(data); err != nil {
		return nil, err
	}

	var verPkt sshfx.VersionPacket
	errch := make(chan error, 1)

	go func() {
		defer close(errch)

		b := make([]byte, maxPacket)

		if err := verPkt.ReadFrom(c.rd, b, maxPacket); err != nil {
			errch <- err
			return
		}

		if verPkt.Version != sftpProtocolVersion {
			errch <- fmt.Errorf("sftp: unexpected server version: got %v, want %v", verPkt.Version, sftpProtocolVersion)
			return
		}
	}()

	select {
	case err := <-errch:
		if err != nil {
			return nil, err
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	exts := make(map[string]string)
	for _, ext := range verPkt.Extensions {
		exts[ext.Name] = ext.Data
	}
	return exts, nil
}

func (c *clientConn) getChan(reqid uint32) (chan<- result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch, loaded := c.inflight[reqid]
	delete(c.inflight, reqid)

	return ch, loaded
}

func (c *clientConn) Wait() error {
	<-c.closed
	return c.err
}

func (c *clientConn) disconnect(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.err = err
	select {
	case <-c.closed:
		// already closed
		return
	default:
	}
	close(c.closed)

	c.resPool.Close() // close and wait for inflight calls to end

	bcastRes := result{
		err: sshfx.StatusConnectionLost,
	}

	for reqid, ch := range c.inflight {
		ch <- bcastRes

		// Replace the chan inflight,
		// we have hijacked this chan,
		// and this guarantees always-only-once sending.
		c.inflight[reqid] = make(chan<- result, 1)
	}
}

func (c *clientConn) recvLoop(maxPacket uint32) error {
	defer c.wr.Close()

	for {
		raw := c.pktPool.Get()
		hint := c.bufPool.Get()

		/* if len(hint) < 64 {
			hint = make([]byte, maxPacket)
		} //*/

		res := result{
			pkt: raw,
		}

		if err := res.pkt.ReadFrom(c.rd, hint, maxPacket); err != nil {
			// Do we plumb a context into this?
			return err
		}

		ch, loaded := c.getChan(res.pkt.RequestID)
		if !loaded {
			// This is an unexpected occurrence.
			// Send the error back to all listeners,
			// so they can terminate gracefully.
			return fmt.Errorf("request id not found: %d", res.pkt.RequestID)
		}

		ch <- res
	}
}

// dispatch will marshal, then dispatch the given request packet.
// Packets are written atomically to the connection.
// It returns the allocated request id (a monotonously incrementing value),
// and either a channel upon which the result will be returned, or an error.
//
// If the cancel channel has been closed before the request is dipatched,
// then dispatch will return an [fs.ErrClosed] error.
func (c *clientConn) dispatch(cancel <-chan struct{}, req sshfx.PacketMarshaller) (uint32, chan result, error) {
	reqid := c.reqid.Add(1)

	header, payload, err := req.MarshalPacket(reqid, c.bufPool.Get())
	if err != nil {
		return reqid, nil, err
	}
	defer c.bufPool.Put(header)

	// payload by design of the API is all but guaranteed to alias a caller-held byte slice,
	// so, _do not_ put it into the bufPool.

	ch, ok := c.resPool.Get()
	if !ok {
		return reqid, nil, sshfx.StatusConnectionLost
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-cancel:
		c.resPool.Put(ch)
		return reqid, nil, fs.ErrClosed
	default:
	}

	if c.inflight == nil {
		c.inflight = make(map[uint32]chan<- result)
	}

	c.inflight[reqid] = ch

	if _, err := c.wr.Write(header); err != nil {
		c.resPool.Put(ch)
		return reqid, nil, fmt.Errorf("sftp: write packet header: %w", err)
	}

	if len(payload) != 0 {
		if _, err := c.wr.Write(payload); err != nil {
			c.resPool.Put(ch)
			return reqid, nil, fmt.Errorf("sftp: write packet payload: %w", err)
		}
	}

	return reqid, ch, nil
}

func (c *clientConn) returnRaw(raw *sshfx.RawPacket) {
	c.bufPool.Put(raw.Data.HintReturn())
	c.pktPool.Put(raw)
}

func (c *clientConn) discardBlocking(ch chan result) {
	res := <-ch

	c.returnRaw(res.pkt)
	c.resPool.Put(ch)
}

func (c *clientConn) discard(ch chan result) {
	select {
	case res := <-ch:
		// We received a result, so we can reuse this channel now.
		c.returnRaw(res.pkt)
		c.resPool.Put(ch)

	default:
		// There wasn't a result immediately,
		// So, to be safe, we will throw away the old result channel.
		// If we tried to reuse this channel,
		// a new request could get an old result.
		c.resPool.Put(make(chan result, 1))
	}
}

func (c *clientConn) recv(ctx context.Context, reqid uint32, ch chan result) (*sshfx.RawPacket, error) {
	select {
	case <-ctx.Done():
		c.discard(ch)
		return nil, ctx.Err()

	case res := <-ch:
		c.resPool.Put(ch)

		if res.err != nil {
			return nil, res.err
		}

		if res.pkt.RequestID != reqid {
			return nil, fmt.Errorf("unexpected request id: %d != %d", res.pkt.RequestID, reqid)
		}

		return res.pkt, nil
	}
}

func (c *clientConn) send(ctx context.Context, cancel <-chan struct{}, req sshfx.PacketMarshaller) (*sshfx.RawPacket, error) {
	reqid, ch, err := c.dispatch(cancel, req)
	if err != nil {
		return nil, err
	}

	return c.recv(ctx, reqid, ch)
}

// ClientOption specifies an optional that can be set on a client.
type ClientOption func(*Client) error

// WithMaxInflight sets the maximum number of inflight packets at one time.
//
// It will generate an error if one attempts to set it to a value less than one.
func WithMaxInflight(count int) ClientOption {
	return func(cl *Client) error {
		if count < 1 {
			return fmt.Errorf("max inflight packets cannot be less than 1, was: %d", count)
		}

		cl.maxInflight = count

		return nil
	}
}

// WithMaxDataLength sets the maximum length of a data that will be used in SSH_FX_READ and SSH_FX_WRITE requests.
// This will also adjust the maximum packet length to at least the data length + 1232 bytes as overhead room.
// (This is the difference between the 34000 byte packet size vs 32768 data packet size.)
//
// The maximum data length can only be increased,
// if an attempt is made to set this value lower than it currently is,
// it will simply not perform any operation.
//
// It will generate an error if one attempts to set the length beyond the 2^32-1 limitation of the sftp protocol.
// There may also be compatibility issues if setting the value above 2^31-1.
func WithMaxDataLength(length int) ClientOption {
	withPktLen := WithMaxPacketLength(length + sshfx.MaxPacketLengthOverhead)

	return func(cl *Client) error {
		if err := withPktLen(cl); err != nil {
			return err
		}

		// This has to be cast to int64 to safely perform this test on 32-bit archs.
		// It should be identified as always false, and elided for them anyways.
		if int64(length) > math.MaxUint32 {
			return fmt.Errorf("sftp: max data length must fit in a uint32: %d", length)
		}

		if int64(length) > math.MaxInt {
			return fmt.Errorf("sftp: max data length must fit in a int: %d", length)
		}

		// Negative values will be stomped by the max with cl.maxDataLen.
		cl.maxDataLen = max(cl.maxDataLen, length)

		return nil
	}
}

// WithMaxPacketLength sets the maximum length of a packet that the client will accept.
//
// The maximum packet length can only be increased,
// if an attempt is made to set this value lower than it currently is,
// it will simply not perform any operation.
func WithMaxPacketLength(length int) ClientOption {
	return func(cl *Client) error {

		// This has to be cast to int64 to safely perform this test on 32-bit archs.
		// It should be identified as always false, and elided for them anyways.
		if int64(length) > math.MaxUint32 {
			return fmt.Errorf("sftp: max packet length must fit in a uint32: %d", length)
		}

		if int64(length) > math.MaxInt {
			return fmt.Errorf("sftp: max packet length must fit in a int: %d", length)
		}

		if length < 0 {
			// Short circuit to avoid a negative value handling during the cast to uint32.
			return nil
		}

		cl.maxPacket = max(cl.maxPacket, uint32(length))
		return nil
	}
}

// Client represents an SFTP session on a *ssh.ClientConn SSH connection.
// Multiple clients can be active on a single SSH connection,
// and a client may be called concurrently from multiple goroutines.
type Client struct {
	conn clientConn

	maxPacket   uint32
	maxDataLen  int
	maxInflight int

	exts map[string]string
}

type respPacket[PKT any] interface {
	*PKT
	sshfx.Packet
}

func getPacket[PKT any, P respPacket[PKT]](ctx context.Context, cancel <-chan struct{}, cl *Client, req sshfx.PacketMarshaller) (*PKT, error) {
	raw, err := cl.conn.send(ctx, cancel, req)
	if err != nil {
		return nil, err
	}
	defer cl.conn.returnRaw(raw)

	var resp P

	switch raw.PacketType {
	case resp.Type():
		resp = new(PKT)
		if err := resp.UnmarshalPacketBody(&raw.Data); err != nil {
			return nil, err
		}

		return resp, nil

	case sshfx.PacketTypeStatus:
		var status sshfx.StatusPacket
		if err := status.UnmarshalPacketBody(&raw.Data); err != nil {
			return nil, err
		}

		return nil, statusToError(&status, false)

	default:
		return nil, fmt.Errorf("unexpected packet type: %s", raw.PacketType)
	}
}

func statusToError(status *sshfx.StatusPacket, okExpected bool) error {
	switch status.StatusCode {
	case sshfx.StatusOK:
		if !okExpected {
			return fmt.Errorf("unexpected SSH_FX_OK")
		}
		return nil

	case sshfx.StatusEOF:
		return io.EOF
	case sshfx.StatusNoSuchFile:
		return fs.ErrNotExist
	case sshfx.StatusPermissionDenied:
		return fs.ErrPermission
	}

	return status
}

func (cl *Client) sendPacket(ctx context.Context, cancel <-chan struct{}, req sshfx.PacketMarshaller) error {
	reqid, ch, err := cl.conn.dispatch(cancel, req)
	if err != nil {
		return err
	}

	var resp sshfx.StatusPacket
	return cl.recvStatus(ctx, reqid, ch, &resp)
}

func (cl *Client) recvStatus(ctx context.Context, reqid uint32, ch chan result, resp *sshfx.StatusPacket) error {
	raw, err := cl.conn.recv(ctx, reqid, ch)
	if err != nil {
		return err
	}
	defer cl.conn.returnRaw(raw)

	switch raw.PacketType {
	case sshfx.PacketTypeStatus:
		if err := resp.UnmarshalPacketBody(&raw.Data); err != nil {
			return err
		}

		return statusToError(resp, true)

	default:
		return fmt.Errorf("unexpected packet type: %s", raw.PacketType)
	}
}

func (cl *Client) sendRead(ctx context.Context, cancel <-chan struct{}, req *sshfx.ReadPacket, resp *sshfx.DataPacket) (int, error) {
	reqid, ch, err := cl.conn.dispatch(cancel, req)
	if err != nil {
		return 0, err
	}

	return cl.recvData(ctx, reqid, ch, resp)
}

func (cl *Client) recvData(ctx context.Context, reqid uint32, ch chan result, resp *sshfx.DataPacket) (int, error) {
	raw, err := cl.conn.recv(ctx, reqid, ch)
	if err != nil {
		return 0, err
	}
	defer cl.conn.returnRaw(raw)

	switch raw.PacketType {
	case sshfx.PacketTypeData:
		err := resp.UnmarshalPacketBody(&raw.Data)
		return len(resp.Data), err

	case sshfx.PacketTypeStatus:
		var status sshfx.StatusPacket
		if err := status.UnmarshalPacketBody(&raw.Data); err != nil {
			return 0, err
		}

		return 0, statusToError(&status, false)

	default:
		return 0, fmt.Errorf("sftp: unexpected packet type: %s", raw.PacketType)
	}
}

func (cl *Client) getDataBuf(size int) []byte {
	hint := cl.conn.bufPool.Get()

	for len(hint) < size {
		hint = cl.conn.bufPool.Get()
		if len(hint) == 0 {
			// Give up, make a new slice, and just throw away all the too small buffers.
			return make([]byte, size, cl.maxPacket) // alloc a new one
		}
	}

	return hint[:size] // trim our buffer to length, it might be longer than chunkSize.
}

// NewClient creates a new SFTP client on conn.
// The context is only used during initialization, and handshake.
func NewClient(ctx context.Context, conn *ssh.Client, opts ...ClientOption) (*Client, error) {
	s, err := conn.NewSession()
	if err != nil {
		return nil, err
	}

	if err := s.RequestSubsystem("sftp"); err != nil {
		s.Close()
		return nil, err
	}

	w, err := s.StdinPipe()
	if err != nil {
		s.Close()
		return nil, err
	}

	r, err := s.StdoutPipe()
	if err != nil {
		s.Close()
		return nil, err
	}

	return NewClientPipe(ctx, r, w, opts...)
}

// NewClientPipe creates a new SFTP client given a Reader and WriteCloser.
// This can be used for connecting an SFTP server over TCP/TLS, or by using the system's ssh client program.
//
// The given context is only used for the negotiation of init and version packets.
func NewClientPipe(ctx context.Context, rd io.Reader, wr io.WriteCloser, opts ...ClientOption) (*Client, error) {
	cl := &Client{
		conn: clientConn{
			rd:     rd,
			wr:     wr,
			closed: make(chan struct{}),
		},

		maxPacket:   sshfx.DefaultMaxPacketLength,
		maxDataLen:  sshfx.DefaultMaxDataLength,
		maxInflight: 64,
	}

	for _, opt := range opts {
		if err := opt(cl); err != nil {
			return nil, err
		}
	}

	exts, err := cl.conn.handshake(ctx, cl.maxPacket)
	if err != nil {
		return nil, err
	}

	cl.exts = exts

	cl.conn.resPool = sync.NewWorkPool[result](cl.maxInflight)

	cl.conn.bufPool = sync.NewSlicePool[[]byte](cl.maxInflight, int(cl.maxPacket))
	cl.conn.pktPool = sync.NewPool[sshfx.RawPacket](cl.maxInflight)

	go func() {
		if err := cl.conn.recvLoop(cl.maxPacket); err != nil {
			cl.conn.disconnect(err)
		}
	}()

	return cl, nil
}

// ReportPoolMetrics writes buffer pool metrics to the given writer, if pool metrics are enabled.
// It is expected that this is only useful during testing, and benchmarking.
//
// To enable you must include `-tag sftp.sync.metrics` to your go command-line.
func (cl *Client) ReportPoolMetrics(wr io.Writer) {
	if cl.conn.bufPool != nil {
		hits, total := cl.conn.bufPool.Hits()

		fmt.Printf("bufpool hit rate: %d / %d = %f\n", hits, total, float64(hits)/float64(total))
	}
}

// Close closes the SFTP session.
func (cl *Client) Close() error {
	cl.conn.disconnect(nil)
	cl.conn.wr.Close()
	return nil
}

func wrapPathError(op, path string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, io.EOF) {
		// Numerous odd things break if we don't return bare io.EOF errors.
		return io.EOF
	}

	return &fs.PathError{Op: op, Path: path, Err: err}
}

func wrapLinkError(op, oldpath, newpath string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, io.EOF) {
		// Numerous odd things break if we don't return bare io.EOF errors.
		return io.EOF
	}

	return &os.LinkError{Op: op, Old: oldpath, New: newpath, Err: err}
}

// Mkdir creates the specified directory.
// An error will be returned if a file or directory with the specified path already exists,
// or if the directory's parent folder does not exist.
func (cl *Client) Mkdir(name string, perm fs.FileMode) error {
	return wrapPathError("mkdir", name,
		cl.sendPacket(context.Background(), nil, &sshfx.MkdirPacket{
			Path: name,
			Attrs: sshfx.Attributes{
				Flags:       sshfx.AttrPermissions,
				Permissions: sshfx.FileMode(perm.Perm()),
			},
		}),
	)
}

// MkdirAll creates a directory named path, along with any necessary parents.
// If a path is already a directory, MkdirAll does nothing and returns nil.
func (cl *Client) MkdirAll(name string, perm fs.FileMode) error {
	// Fast path: if we can tell whether name is a directory or file, stop with success or error.
	dir, err := cl.Stat(name)
	if err == nil {
		if dir.IsDir() {
			return nil
		}

		return wrapPathError("mkdir", name, syscall.ENOTDIR)
	}

	// Slow path: make sure parent exists and then call Mkdir for name.

	if parent := path.Dir(name); parent != "" {
		err = cl.MkdirAll(parent, perm)
		if err != nil {
			return err
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	err = cl.Mkdir(name, perm)
	if err != nil {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := cl.LStat(name)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}

	return nil
}

// Remove removes the named file or (empty) directory.
//
// If both operations fail, then Remove will stat the named filesystem object.
// It then returns the error from that SSH_FX_STAT request if one occurs,
// or the error from the SSH_FX_RMDIR request if it is a directory,
// otherwise returning the error from the SSH_FX_REMOVE request.
func (cl *Client) Remove(name string) error {
	ctx := context.Background()

	err := cl.sendPacket(ctx, nil, &sshfx.RemovePacket{
		Path: name,
	})
	if err == nil {
		return nil
	}

	err1 := cl.sendPacket(ctx, nil, &sshfx.RmdirPacket{
		Path: name,
	})
	if err1 == nil {
		return nil
	}

	// Both failed: figure out which error to return.
	if err != err1 {
		attrs, err2 := getPacket[sshfx.AttrsPacket](ctx, nil, cl, &sshfx.StatPacket{
			Path: name,
		})
		if err2 != nil {
			err = err2
		} else {
			if perm, ok := attrs.Attrs.GetPermissions(); ok && perm.IsDir() {
				err = err1
			}
		}
	}

	return wrapPathError("remove", name, err)
}

func (cl *Client) setstat(ctx context.Context, name string, attrs *sshfx.Attributes) error {
	return wrapPathError("setstat", name,
		cl.sendPacket(ctx, nil, &sshfx.SetStatPacket{
			Path:  name,
			Attrs: *attrs,
		}),
	)
}

// Truncate changes the size of the named file.
// If the file is a symbolic link, it changes the size of the link's target.
func (cl *Client) Truncate(name string, size int64) error {
	return cl.setstat(context.Background(), name, &sshfx.Attributes{
		Flags: sshfx.AttrSize,
		Size:  uint64(size),
	})
}

// Chmod changes the mode of the named file to mode.
// If the file is a symbolic link, it changes the mode of the link's target.
//
// The Go FileMdoe, will be converted to a "portable" POSIX file permission, and then sent to the server.
// The server is then responsible for interpreting that permission.
// It is possible the server and this client disagree on what some flags mean.
func (cl *Client) Chmod(name string, mode fs.FileMode) error {
	return cl.setstat(context.Background(), name, &sshfx.Attributes{
		Flags:       sshfx.AttrPermissions,
		Permissions: sshfx.FromGoFileMode(mode),
	})
}

// Chown changes the numeric uid and gid of the named file.
// If the file is a symbolic link, it changes the uid and gid of the link's target.
//
// [os.Chown] provides that a uid or gid of -1 means to not change that value,
// but we cannot guarantee the same semantics here.
// The server is told to set the uid and gid as given, and it is up to the server to define that behavior.
func (cl *Client) Chown(name string, uid, gid int) error {
	return cl.setstat(context.Background(), name, &sshfx.Attributes{
		Flags: sshfx.AttrUIDGID,
		UID:   uint32(uid),
		GID:   uint32(gid),
	})
}

// Chtimes changes the access and modification times of the named file,
// similar to the Unix utime() or utimes() functions.
//
// The SFTP protocol only supports an accuracy to the second,
// so these times will be truncated to the second before being sent to the server.
// The server may additional truncate or round the values to an even less precise time unit.
//
// [os.Chtimes] provides that a zero [time.Time] value will leave the corresponding file time unchanged,
// but we cannot guarantee the same semantics here.
// The server is told to set the atime and mtime as given,
// and it is up to the server to define that behavior.
func (cl *Client) Chtimes(name string, atime, mtime time.Time) error {
	return cl.setstat(context.Background(), name, &sshfx.Attributes{
		Flags: sshfx.AttrACModTime,
		ATime: uint32(atime.Unix()),
		MTime: uint32(mtime.Unix()),
	})
}

// RealPath returns the server canonicalized absolute path for the given path name.
// This is useful for converting path names containing ".." components,
// or relative pathnames without a leading slash into absolute paths.
func (cl *Client) RealPath(name string) (string, error) {
	pkt, err := getPacket[sshfx.PathPseudoPacket](context.Background(), nil, cl, &sshfx.RealPathPacket{
		Path: name,
	})
	if err != nil {
		return "", wrapPathError("realpath", name, err)
	}

	return pkt.Path, nil
}

// ReadLink returns the destination of the named symbolic link.
//
// The client cannot guarantee any specific way that a server handles a relative link destination.
// That is, you may receive a relative link destination, one that has been converted to an absolute path.
func (cl *Client) ReadLink(name string) (string, error) {
	pkt, err := getPacket[sshfx.PathPseudoPacket](context.Background(), nil, cl, &sshfx.ReadLinkPacket{
		Path: name,
	})
	if err != nil {
		return "", wrapPathError("readlink", name, err)
	}

	return pkt.Path, nil
}

// Rename renames (moves) oldpath to newpath.
// If newpath already exists and is not a directory, Rename replaces it.
// Server-specific restrictions may apply when old path and new path are in different directories.
// Even within the same directory, on non-Unix servers Rename is not guaranteed to be an atomic operation.
func (cl *Client) Rename(oldpath, newpath string) error {
	if cl.hasExtension(openssh.ExtensionPOSIXRename()) {
		return wrapLinkError("rename", oldpath, newpath,
			cl.sendPacket(context.Background(), nil, &openssh.POSIXRenameExtendedPacket{
				OldPath: oldpath,
				NewPath: newpath,
			}),
		)
	}

	return wrapLinkError("rename", oldpath, newpath,
		cl.sendPacket(context.Background(), nil, &sshfx.RenamePacket{
			OldPath: oldpath,
			NewPath: newpath,
		}),
	)
}

// Symlink creates newname as a symbolic link to oldname.
// There is no guarantee for how a server may handle the request if oldname does not exist.
func (cl *Client) Symlink(oldname, newname string) error {
	return wrapLinkError("symlink", oldname, newname,
		cl.sendPacket(context.Background(), nil, &sshfx.SymlinkPacket{
			LinkPath:   newname,
			TargetPath: oldname,
		}),
	)
}

func (cl *Client) hasExtension(ext *sshfx.ExtensionPair) bool {
	return cl.exts[ext.Name] == ext.Data
}

// Link creates newname as a hard link to oldname file.
//
// If the server did not announce support for the "hardlink@openssh.com" extension,
// then no request will be sent,
// and Link returns an *fs.LinkError wrapping sshfx.StatusOpUnsupported.
func (cl *Client) Link(oldname, newname string) error {
	if !cl.hasExtension(openssh.ExtensionHardlink()) {
		return wrapLinkError("hardlink", oldname, newname, sshfx.StatusOpUnsupported)
	}

	return wrapLinkError("hardlink", oldname, newname,
		cl.sendPacket(context.Background(), nil, &openssh.HardlinkExtendedPacket{
			OldPath: oldname,
			NewPath: newname,
		}),
	)
}

// Readdir reads the named directory, returning all its directory entries as [fs.FileInfo] sorted by filename.
// If an error occurs reading the directory,
// Readdir returns the entries it was able to read before the error, along with the error.
func (cl *Client) Readdir(name string) ([]fs.FileInfo, error) {
	d, err := cl.OpenDir(name)
	if err != nil {
		return nil, err
	}
	defer d.Close()

	fis, err := d.Readdir(0)

	slices.SortFunc(fis, func(a, b fs.FileInfo) int {
		return cmp.Compare(a.Name(), b.Name())
	})

	return fis, err
}

// ReadDir reads the named directory, returning all its directory entries sorted by filename.
// If an error occurs reading the directory,
// ReadDir returns the entries it was able to read before the error, along with the error.
func (cl *Client) ReadDir(name string) ([]fs.DirEntry, error) {
	return cl.ReadDirContext(context.Background(), name)
}

// ReadDirContext reads the named directory, returning all its directory entries sorted by filename.
// If an error occurs reading the directory, including the context being canceled,
// ReadDir returns the entries is was able to read before the error, along with the error.
func (cl *Client) ReadDirContext(ctx context.Context, name string) ([]fs.DirEntry, error) {
	d, err := cl.OpenDir(name)
	if err != nil {
		return nil, err
	}
	defer d.Close()

	fis, err := d.ReadDir(0)

	slices.SortFunc(fis, func(a, b fs.DirEntry) int {
		return cmp.Compare(a.Name(), b.Name())
	})

	return fis, err
}

// Stat returns a FileInfo describing the named file.
// If the file is a symbolic link, the returned FileInfo describes the link's target.
func (cl *Client) Stat(name string) (fs.FileInfo, error) {
	pkt, err := getPacket[sshfx.AttrsPacket](context.Background(), nil, cl, &sshfx.StatPacket{
		Path: name,
	})
	if err != nil {
		return nil, wrapPathError("stat", name, err)
	}

	return &sshfx.NameEntry{
		Filename: name,
		Attrs:    pkt.Attrs,
	}, nil
}

// LStat returns a FileInfo describing the named file.
// If the file is a symbolic link, the returned FileInfo describes the symbolic link
// LStat makes no attempte to follow the link.
//
// The description returned may have server specific caveats and special cases that cannot be covered here.
func (cl *Client) LStat(name string) (fs.FileInfo, error) {
	pkt, err := getPacket[sshfx.AttrsPacket](context.Background(), nil, cl, &sshfx.LStatPacket{
		Path: name,
	})
	if err != nil {
		return nil, wrapPathError("lstat", name, err)
	}

	return &sshfx.NameEntry{
		Filename: name,
		Attrs:    pkt.Attrs,
	}, nil
}

type handle struct {
	value  atomic.Pointer[string]
	closed chan struct{}
}

func (h *handle) init(handle string) {
	h.value.Store(&handle)
	h.closed = make(chan struct{})
}

func (h *handle) get() (handle string, cancel <-chan struct{}, err error) {
	p := h.value.Load()
	if p == nil {
		return "", nil, fs.ErrClosed
	}
	return *p, h.closed, nil
}

func (h *handle) close(cl *Client) error {
	// The design principle here is that when `openssh-portable/sftp-server.c` is doing `handle_close`,
	// it will unconditionally mark the handle as unused,
	// so we need to also unconditionally mark this handle as invalid.
	// By invalidating our local copy of the handle,
	// we ensure that there cannot be any new erroneous use-after-close receiver methods started after this swap.
	handle := h.value.Swap(nil)
	if handle == nil {
		return fs.ErrClosed
	}

	// The atomic Swap above ensures that only one Close can ever get here.
	// We could also use a mutex to guarantee exclusivity here,
	// but that would block Close until all synchronized operations have completed,
	// some of which could be paused indefinitely.
	//
	// See: https://github.com/pkg/sftp/issues/603 for more details.

	// So, we have defended now against new receiver methods starting,
	// but since an outstanding method could still be holding the handle, we still need a close signal.
	// Since this close HAPPENS BEFORE the sendPacket below,
	// this ensures that after closing this channel, no further requests will be dispatched.
	// Meaning we know that the close request below will be the final request from this handle.
	close(h.closed)

	// One might assume we could just simply use the closed channel alone,
	// but because close panics if called twice, we need a select to test if the channel is already closed,
	// and since there is a window of time between such a test and the close, two goroutines can race.
	// So we still need to synchronize the close operation anyways, so either atomic pointer or mutex.

	// It should be obvious, but do not pass h.closed into this sendPacket, or it will never be sent.
	// Less obviously, DO NOT pipe a context through this function to the sendPacket.
	// We want to ensure that even in a closed-context codepath, that the SSH_FXP_CLOSED packet is still sent.
	return cl.sendPacket(context.Background(), nil, &sshfx.ClosePacket{
		Handle: *handle,
	})
}

// Dir represents an open directory handle.
//
// The methods of Dir are safe for concurrent use.
type Dir struct {
	cl   *Client
	name string

	handle handle

	mu      sync.RWMutex
	entries []*sshfx.NameEntry
}

// OpenDir opens the named directory for reading.
// If successful, methods on the returned Dir can be used for reading.
//
// The semantics of SSH_FX_OPENDIR is such that the associated file handle is in a read-only mode.
func (cl *Client) OpenDir(name string) (*Dir, error) {
	pkt, err := getPacket[sshfx.HandlePacket](context.Background(), nil, cl, &sshfx.OpenDirPacket{
		Path: name,
	})
	if err != nil {
		return nil, wrapPathError("opendir", name, err)
	}

	d := &Dir{
		cl:   cl,
		name: name,
	}

	d.handle.init(pkt.Handle)

	return d, nil
}

func (d *Dir) wrapErr(op string, err error) error {
	return wrapPathError(op, d.name, err)
}

// Close closes the Dir, rendering it unusable for I/O.
// Close will not send any request, and return an error if it has already been called.
func (d *Dir) Close() error {
	if d == nil {
		return os.ErrInvalid
	}

	return d.wrapErr("close", d.handle.close(d.cl))
}

// Name returns the name of the directory as presented to OpenDir.
func (d *Dir) Name() string {
	return d.name
}

// rangedir returns an iterator over the directory entries of the directory.
// We do not expose an iterator, because none has been standardized yet.
// and we do not want to accidentally implement an inconsistent API.
// However, for internal usage, we can definitely make use of this to simplify the common parts of ReadDir and Readdir.
//
// Callers must guarantee synchronization by either holding the file lock, or holding an exclusive reference.
func (d *Dir) rangedir(ctx context.Context) iter.Seq2[*sshfx.NameEntry, error] {
	return func(yield func(v *sshfx.NameEntry, err error) bool) {
		// Pull from saved entries first.
		for i, ent := range d.entries {
			if !yield(ent, nil) {
				// Early break, delete the entries we have yielded.
				d.entries = slices.Delete(d.entries, 0, i+1)
				return
			}
		}

		// We got through all the remaining entries, delete all the entries.
		d.entries = slices.Delete(d.entries, 0, len(d.entries))

		for {
			handle, closed, err := d.handle.get()
			if err != nil {
				yield(nil, err)
				return
			}

			pkt, err := getPacket[sshfx.NamePacket](ctx, closed, d.cl, &sshfx.ReadDirPacket{
				Handle: handle,
			})
			if err != nil {
				// There are no remaining entries to save here,
				// SFTP can only return either an error or a result, never both.
				yield(nil, err)
				return
			}

			for i, entry := range pkt.Entries {
				if !yield(entry, nil) {
					// Early break, save the remaining entries we got for maybe later.
					d.entries = append(d.entries, pkt.Entries[i+1:]...)
					return
				}
			}
		}
	}
}

// Readdir calls [ReaddirContext] with the background context.
func (d *Dir) Readdir(n int) ([]fs.FileInfo, error) {
	return d.ReaddirContext(context.Background(), n)
}

// ReaddirContext reads the contents of the directory and returns a slice of up to n [fs.FileInfo] values,
// as they were returned from the server,
// in directory order.
// Subsequent calls to the same file will yield later FileInfo records in the directory.
//
// If n > 0, ReaddirContext returns as most n FileInfo records.
// In this case, if ReadDirContext returns an empty slice,
// it will return an error explaining why.
// At the end of a directory, the error is io.EOF.
//
// If n <= 0, ReaddirContext returns all the FileInfo records remaining in the directory.
// When it succeeds, it returns a nil error (not io.EOF).
func (d *Dir) ReaddirContext(ctx context.Context, n int) ([]fs.FileInfo, error) {
	if d == nil {
		return nil, os.ErrInvalid
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	var ret []fs.FileInfo

	for ent, err := range d.rangedir(ctx) {
		if err != nil {
			if errors.Is(err, io.EOF) && n <= 0 {
				return ret, nil
			}

			return ret, d.wrapErr("readdir", err)
		}

		ret = append(ret, ent)

		if n > 0 && len(ret) >= n {
			break
		}
	}

	return ret, nil
}

// ReadDir calls [ReadDirContext] with the background context.
func (d *Dir) ReadDir(n int) ([]fs.DirEntry, error) {
	return d.ReadDirContext(context.Background(), n)
}

// ReadDirContext reads the contents of the directory and returns a slice of up to n [fs.DirEntry] values,
// as they were returned from the server,
// in directory order.
// Subsequent calls to the same file will yield later DirEntry records in the directory.
//
// If n > 0, ReadDirContext returns as most n DirEntry records.
// In this case, if ReadDirContext returns an empty slice,
// it will return an error explaining why.
// At the end of a directory, the error is io.EOF.
//
// If n <= 0, ReadDirContext returns all the DirEntry records remaining in the directory.
// When it succeeds, it returns a nil error (not io.EOF).
func (d *Dir) ReadDirContext(ctx context.Context, n int) ([]fs.DirEntry, error) {
	if d == nil {
		return nil, os.ErrInvalid
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	var ret []fs.DirEntry

	for ent, err := range d.rangedir(ctx) {
		if err != nil {
			if errors.Is(err, io.EOF) && n <= 0 {
				return ret, nil
			}

			return ret, d.wrapErr("readdir", err)
		}

		ret = append(ret, ent)

		if n > 0 && len(ret) >= n {
			break
		}
	}

	return ret, nil
}

// File represents an open file handle.
//
// The methods of File are safe for concurrent use.
type File struct {
	cl   *Client
	name string

	handle handle

	mu     sync.RWMutex
	offset int64 // current offset within remote file
}

// These aliases to the os package values are provided as a convenience to avoid needing two imports to use OpenFile.
const (
	// Exactly one of OpenReadOnly, OpenWriteOnly, OpenReadWrite must be specified.
	OpenFlagReadOnly  = os.O_RDONLY
	OpenFlagWriteOnly = os.O_WRONLY
	OpenFlagReadWrite = os.O_RDWR
	// The remaining values may be or'ed in to control behavior.
	OpenFlagAppend    = os.O_APPEND
	OpenFlagCreate    = os.O_CREATE
	OpenFlagTruncate  = os.O_TRUNC
	OpenFlagExclusive = os.O_EXCL
)

// toPortableFlags converts the flags passed to OpenFile into SFTP flags.
// Unsupported flags are ignored.
func toPortableFlags(f int) uint32 {
	var out uint32
	switch f & (OpenFlagReadOnly | OpenFlagWriteOnly | OpenFlagReadWrite) {
	case OpenFlagReadOnly:
		out |= sshfx.FlagRead
	case OpenFlagWriteOnly:
		out |= sshfx.FlagWrite
	case OpenFlagReadWrite:
		out |= sshfx.FlagRead | sshfx.FlagWrite
	}
	if f&OpenFlagAppend == OpenFlagAppend {
		out |= sshfx.FlagAppend
	}
	if f&OpenFlagCreate == OpenFlagCreate {
		out |= sshfx.FlagCreate
	}
	if f&OpenFlagTruncate == OpenFlagTruncate {
		out |= sshfx.FlagTruncate
	}
	if f&OpenFlagExclusive == OpenFlagExclusive {
		out |= sshfx.FlagExclusive
	}
	return out
}

// Open opens the named file for reading.
// If successful, methods on the returned file can be used for reading;
// the associated file handle has mode OpenFlagReadOnly.
func (cl *Client) Open(name string) (*File, error) {
	return cl.OpenFile(name, OpenFlagReadOnly, 0)
}

// Create creates of truncates the named file.
// If the file already exists, it is truncated.
// If the file does not exist, it is created with mode 0o666 (before umask).
// If successful, methods on the returned File can be used for I/O;
// the associated file handle has mode OpenFlagReadWrite.
func (cl *Client) Create(name string) (*File, error) {
	return cl.OpenFile(name, OpenFlagReadWrite|OpenFlagCreate|OpenFlagTruncate, 0666)
}

// OpenFile is the generalized open call;
// most users can use the simplified Open or Create methods instead.
// It opens the named file with the specified flag (OpenFlagReadOnly, etc.).
// If the file does not exist, and the OpenFileCreate flag is passed, it is created with mode perm (before umask).
// If successful, methods on the returned File can be used for I/O.
//
// Note well: since all Write operations are down through an offset-specifying operation,
// the OpenFlagAppend flag is currently ignored.
func (cl *Client) OpenFile(name string, flag int, perm fs.FileMode) (*File, error) {
	pkt, err := getPacket[sshfx.HandlePacket](context.Background(), nil, cl, &sshfx.OpenPacket{
		Filename: name,
		PFlags:   toPortableFlags(flag),
		Attrs: sshfx.Attributes{
			Flags:       sshfx.AttrPermissions,
			Permissions: sshfx.FileMode(perm.Perm()),
		},
	})
	if err != nil {
		return nil, wrapPathError("openfile", name, err)
	}

	f := &File{
		cl:   cl,
		name: name,
	}

	f.handle.init(pkt.Handle)

	return f, nil
}

func (f *File) wrapErr(op string, err error) error {
	return wrapPathError(op, f.name, err)
}

// Close closes the File, rendering it unusable for I/O.
// Close will not send any request, and return an error if it has already been called.
func (f *File) Close() error {
	if f == nil {
		return fs.ErrInvalid
	}

	return f.wrapErr("close", f.handle.close(f.cl))
}

// Name returns the name of the file as presented to Open.
//
// It is safe to call Name after Close.
func (f *File) Name() string {
	return f.name
}

func (f *File) setstat(ctx context.Context, attrs *sshfx.Attributes) error {
	if f == nil {
		return fs.ErrInvalid
	}

	handle, closed, err := f.handle.get()
	if err != nil {
		return f.wrapErr("fsetstat", err)
	}

	return f.wrapErr("fsetstat",
		f.cl.sendPacket(ctx, closed, &sshfx.FSetStatPacket{
			Handle: handle,
			Attrs:  *attrs,
		}),
	)
}

// Truncate changes the size of the file.
// It does not change the I/O offset.
func (f *File) Truncate(size int64) error {
	return f.setstat(context.Background(), &sshfx.Attributes{
		Flags: sshfx.AttrSize,
		Size:  uint64(size),
	})
}

// Chmod changes the mode of the file to mode.
//
// The Go FileMode will be converted to a "portable" POSIX file permission, and then sent to the server.
// The server is then responsible for interpreting that permission.
// It is possible the server and this client disagree on what some flags mean.
func (f *File) Chmod(mode fs.FileMode) error {
	return f.setstat(context.Background(), &sshfx.Attributes{
		Flags:       sshfx.AttrPermissions,
		Permissions: sshfx.FromGoFileMode(mode),
	})
}

// Chown changes the numeric uid and gid of the named file.
// The server is told to set the uid and gid as given, and it is up to the server to define that behavior.
func (f *File) Chown(uid, gid int) error {
	return f.setstat(context.Background(), &sshfx.Attributes{
		Flags: sshfx.AttrUIDGID,
		UID:   uint32(uid),
		GID:   uint32(gid),
	})
}

// Chtimes sends a request to change the access and modification times of the file.
//
// Be careful, the server may later alter the access or modification time upon Close of this file.
// To ensure the times stick, you should Close the file, and then use [Client.Chtimes] to update the times.
func (f *File) Chtimes(atime, mtime time.Time) error {
	return f.setstat(context.Background(), &sshfx.Attributes{
		Flags: sshfx.AttrACModTime,
		ATime: uint32(atime.Unix()),
		MTime: uint32(mtime.Unix()),
	})
}

// Stat returns the FileInfo structure describing file.
func (f *File) Stat() (fs.FileInfo, error) {
	if f == nil {
		return nil, fs.ErrInvalid
	}

	handle, closed, err := f.handle.get()
	if err != nil {
		return nil, f.wrapErr("fstat", err)
	}

	pkt, err := getPacket[sshfx.AttrsPacket](context.Background(), closed, f.cl, &sshfx.FStatPacket{
		Handle: handle,
	})
	if err != nil {
		return nil, f.wrapErr("fstat", err)
	}

	return &sshfx.NameEntry{
		Filename: f.name,
		Attrs:    pkt.Attrs,
	}, nil
}

func (f *File) writeatFull(ctx context.Context, b []byte, off int64) (written int, err error) {
	handle, closed, err := f.handle.get()
	if err != nil {
		return 0, err
	}

	req := &sshfx.WritePacket{
		Handle: handle,
		Offset: uint64(off),
	}

	chunkSize := f.cl.maxDataLen

	for len(b) > 0 {
		n := min(len(b), chunkSize)

		req.Data, b = b[:n], b[n:]

		err = f.cl.sendPacket(ctx, closed, req)
		if err != nil {
			return written, f.wrapErr("writeat", err)
		}

		req.Offset += uint64(n)
		written += n
	}

	return written, nil
}

func (f *File) writeat(ctx context.Context, b []byte, off int64) (written int, err error) {
	if len(b) <= f.cl.maxDataLen {
		// This should be able to be serviced with just 1 request.
		// So, just do it directly.
		return f.writeatFull(ctx, b, off)
	}

	handle, closed, err := f.handle.get()
	if err != nil {
		return 0, f.wrapErr("writeat", err)
	}

	// Split the write into multiple maxPacket sized concurrent writes bounded by maxInflight.
	// This allows writes with a suitably large buffer to transfer data at a much faster rate
	// due to overlapping round trip times.

	type work struct {
		reqid uint32
		res   chan result
		off   uint64
	}
	workCh := make(chan work, f.cl.maxInflight)

	type rwErr struct {
		off uint64
		err error
	}
	errCh := make(chan rwErr)

	sendCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Dispatch: Read and dispatch into any number of Writes of length <= f.cl.maxDataLen.
	go func() {
		defer close(workCh)

		ctx := sendCtx // shadow ctx so we cannot accidentally reference the parent context here.

		b := b
		chunkSize := f.cl.maxDataLen

		req := &sshfx.WritePacket{
			Handle: handle,
			Offset: uint64(f.offset),
		}

		for len(b) > 0 {
			n := min(len(b), chunkSize)

			req.Data, b = b[:n], b[n:]

			reqid, res, err := f.cl.conn.dispatch(closed, req)
			if err != nil {
				errCh <- rwErr{req.Offset, err}
				return
			}

			select {
			case workCh <- work{reqid, res, req.Offset}:
			case <-ctx.Done():
				// We're not sending this as work,
				// so we need to discard result and restore the result pool.
				f.cl.conn.discard(res)

				// Don't send the context error here.
				// We let the reduce code handle any parent context errors.
				return
			}

			req.Offset += uint64(n)
		}
	}()

	// Receive: receive the SSH_FXP_STATUS from each write.
	// We only need the one workCh listener, though.
	// All result channels are len==1 buffered, so we can process them sequentially no problem.
	go func() {
		defer close(errCh)

		var status sshfx.StatusPacket

		for work := range workCh {
			err := f.cl.recvStatus(ctx, work.reqid, work.res, &status)
			if err != nil {
				errCh <- rwErr{work.off, err}

				// DO NOT return.
				// We want to ensure that workCh is drained before errCh is closed.
			}
		}
	}()

	// Reduce: Collect any errors into the earliest offset to return an error.
	var firstErr rwErr
	for rwErr := range errCh {
		if firstErr.err == nil || rwErr.off <= firstErr.off {
			firstErr = rwErr
		}

		// Stop the dispatcher, but do not return yet.
		// We want to collect all the outstanding possible errors.
		cancel()
	}

	if firstErr.err != nil {
		// firstErr.err != nil if and only if firstErr.off is a valid offset.
		//
		// firstErr.off will then be the lesser of:
		// * the offset of the start of the first error received in response to a write packet.
		// * the offset of the start of the first error received dispatching a write packet offset.
		//
		// Either way, this should be the last successfully written offset.
		written := int64(firstErr.off) - f.offset
		f.offset = int64(firstErr.off)

		return int(written), f.wrapErr("writeat", firstErr.err)
	}

	// We didn't hit any errors, so we must have written all the bytes in the buffer.
	written = len(b)
	f.offset += int64(written)

	return written, nil
}

// WriteAt writes len(b) bytes to the File starting at byte offset off.
// It returns the number of bytes written and an error, if any.
// WriteAt returns a non-nil error when n != len(b).
func (f *File) WriteAt(b []byte, off int64) (n int, err error) {
	if f == nil {
		return 0, fs.ErrInvalid
	}

	return f.writeat(context.Background(), b, off)
}

// Write writes len(b) bytes from b to the File.
// It returns the number of bytes written and an error, if any.
// Write returns a non-nil error when n != len(b)
func (f *File) Write(b []byte) (int, error) {
	if f == nil {
		return 0, fs.ErrInvalid
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	n, err := f.writeat(context.Background(), b, f.offset)
	f.offset += int64(n)

	return n, err
}

// WriteString is like Write, but writes the contents of the string s rather than a slice of bytes.
func (f *File) WriteString(s string) (n int, err error) {
	b := unsafe.Slice(unsafe.StringData(s), len(s))
	return f.Write(b)
}

func (f *File) readFromSequential(ctx context.Context, r io.Reader) (read int64, err error) {
	if f == nil {
		return 0, fs.ErrInvalid
	}

	handle, closed, err := f.handle.get()
	if err != nil {
		return 0, f.wrapErr("readfrom", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	b := make([]byte, f.cl.maxDataLen)

	req := &sshfx.WritePacket{
		Handle: handle,
	}

	for {
		n, err := r.Read(b)
		if n < 0 {
			panic("sftp: readfrom: read returned negative count")
		}

		if n > 0 {
			read += int64(n)

			req.Data = b[:n]
			req.Offset = uint64(f.offset)

			err1 := f.cl.sendPacket(ctx, closed, req)
			if err1 == nil {
				// Only increment file offset, if we got a sucess back.
				f.offset += int64(n)
			}

			err = cmp.Or(err, err1)
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return read, nil // return nil instead of EOF
			}

			return read, f.wrapErr("readfrom", err)
		}
	}
}

type panicInstead string

func (e panicInstead) Error() string {
	return string(e)
}

// ReadFrom reads data from r until EOF and writes it to the file.
// The return value is the number of bytes read from the Reader.
// Any error except io.EOF encountered during the read or write is also returned.
//
// This method is prefered over calling Write multiple times
// to maximize throughput when transferring an entire file,
// especially over high-latency links.
func (f *File) ReadFrom(r io.Reader) (read int64, err error) {
	if f == nil {
		return 0, fs.ErrInvalid
	}

	handle, closed, err := f.handle.get()
	if err != nil {
		return 0, f.wrapErr("readfrom", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	ctx := context.Background()
	chunkSize := f.cl.maxDataLen

	type work struct {
		reqid uint32
		res   chan result
		off   uint64
	}
	workCh := make(chan work, f.cl.maxInflight)

	type rwErr struct {
		off uint64
		err error
	}
	errCh := make(chan rwErr)

	sendCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Dispatch: Read and dispatch into any number of Writes of length <= f.cl.maxDataLen.
	go func() {
		defer close(workCh)

		ctx := sendCtx // shadow ctx so we cannot accidentally reference the parent context here.

		b := f.cl.getDataBuf(chunkSize)
		defer f.cl.conn.bufPool.Put(b)

		req := &sshfx.WritePacket{
			Handle: handle,
			Offset: uint64(f.offset),
		}

		for {
			n, err := r.Read(b)
			if n < 0 {
				errCh <- rwErr{req.Offset, panicInstead("sftp: readfrom: read returned negative count")}
				return
			}

			if n > 0 {
				read += int64(n)
				req.Data = b[:n]

				reqid, res, err1 := f.cl.conn.dispatch(closed, req)
				if err1 == nil { // If _NO_ error occurred during dispatch.
					select {
					case workCh <- work{reqid, res, req.Offset}:
					case <-ctx.Done():
						// We're not sending this as work,
						// so we need to discard result and restore the result pool.
						f.cl.conn.discard(res)

						// Don't send the context error here.
						// We let the reduce code handle any parent context errors.
						return
					}

					req.Offset += uint64(n)
				}

				err = cmp.Or(err, err1)
			}

			if err != nil {
				if !errors.Is(err, io.EOF) {
					errCh <- rwErr{req.Offset, err}
				}
				return
			}
		}
	}()

	// Receive: receive the SSH_FXP_STATUS from each write.
	// We only need the one workCh listener, though.
	// All result channels are len==1 buffered, so we can process them sequentially no problem.
	go func() {
		defer close(errCh)

		var status sshfx.StatusPacket

		for work := range workCh {
			err := f.cl.recvStatus(ctx, work.reqid, work.res, &status)
			if err != nil {
				errCh <- rwErr{work.off, err}

				// DO NOT return.
				// We want to ensure that workCh is drained before errCh is closed.
			}
		}
	}()

	// Reduce: Collect any errors into the earliest offset to return an error.
	var firstErr rwErr
	for rwErr := range errCh {
		if firstErr.err == nil || rwErr.off <= firstErr.off {
			firstErr = rwErr
		}

		// Stop the dispatcher, but do not return yet.
		// We want to collect all the outstanding possible errors.
		cancel()
	}

	if firstErr.err != nil {
		// firstErr.err != nil if and only if firstErr.off is a valid offset.
		//
		// firstErr.off will then be the lesser of:
		// * the offset of the first error from writing,
		// * the last successfully read offset.
		//
		// Either way, this should be the last successful write offset.
		f.offset = int64(firstErr.off)

		if pErr, ok := err.(panicInstead); ok {
			// We control this error, so we can safely use a simple type assert here.
			panic(pErr.Error())
		}

		// ReadFrom is defined to return the read bytes, regardless of any write errors.
		return read, f.wrapErr("readfrom", firstErr.err)
	}

	// We didn't hit any errors, so we must have written all the bytes that we read until EOF.
	f.offset += read
	return read, nil
}

// readatFull attempts to read the whole entire length of the buffer from the file starting at the offset.
// It will continue progressively reading into the buffer until it fills the whole buffer, or an error occurs.
//
// This is prefered over io.ReadFull, because it can reuse read and data packet allocations.
func (f *File) readatFull(ctx context.Context, b []byte, off int64) (read int, err error) {
	handle, closed, err := f.handle.get()
	if err != nil {
		return 0, f.wrapErr("readat", err)
	}

	req := &sshfx.ReadPacket{
		Handle: handle,
		Offset: uint64(off),
	}

	var resp sshfx.DataPacket

	chunkSize := f.cl.maxDataLen

	for len(b) > 0 {
		n := min(len(b), chunkSize)

		req.Length = uint32(n)

		// Fun fact: if we get a larger data packet than the hint resp.Data, we helpfully grow it to fit.
		// So, we need to clip our buffer here to ensure we don't accidentally write past len(b) into cap(b).
		// We clip here instead of b at the top, so that we know m > len(rb) must have reallocated.
		// Otherwise, we would need to use unsafe.SliceData to identify a reallocation.
		resp.Data = slices.Clip(b[:n])

		m, err := f.cl.sendRead(ctx, closed, req, &resp)

		if m > n {
			// OH NO! We received more data than we expected!
			// Because of the slices.Clip above, this MUST have reallocated.
			// So we have to copy the data over ourselves.
			m = copy(b, resp.Data) // Maybe copies over more than n bytes.
		}
		b = b[m:]

		req.Offset += uint64(m)
		read += m

		if err != nil {
			return read, f.wrapErr("readat", err)
		}
	}

	return read, nil
}

func (f *File) readat(ctx context.Context, b []byte, off int64) (read int, err error) {
	if len(b) <= f.cl.maxDataLen {
		// This should be able to be serviced most times with only 1 request.
		// So, just do it sequentially.
		return f.readatFull(ctx, b, off)
	}

	handle, closed, err := f.handle.get()
	if err != nil {
		return 0, f.wrapErr("readat", err)
	}

	sendCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type work struct {
		reqid uint32
		res   chan result

		b   []byte
		off uint64
	}
	workCh := make(chan work, f.cl.maxInflight)

	type rwErr struct {
		off uint64
		err error
	}
	errCh := make(chan rwErr)

	// Split the read into multiple maxDataLen-sized concurrent reads.
	// This allows reads with a suitably large buffer to transfer data at a much faster rate
	// by overlapping round trip times.

	// Dispatch: Dispatch into any number of Reads of length <= f.cl.maxDataLen.
	go func() {
		defer close(workCh)

		ctx := sendCtx // shadow ctx so we cannot accidentally reference the parent context here.

		b := b
		chunkSize := f.cl.maxDataLen

		req := &sshfx.ReadPacket{
			Handle: handle,
			Offset: uint64(off),
		}

		for len(b) > 0 {
			n := min(len(b), chunkSize)

			req.Length = uint32(n)

			reqid, res, err := f.cl.conn.dispatch(closed, req)
			if err != nil {
				errCh <- rwErr{req.Offset, err}
				return
			}

			select {
			case workCh <- work{reqid, res, b[:n], req.Offset}:
			case <-ctx.Done():
				// We're not sending this as work,
				// so we need to discard result and restore the result pool.
				f.cl.conn.discard(res)

				// Don't send the context error here.
				// We let the reduce code handle any parent context errors.
				return
			}

			b = b[n:]
			req.Offset += uint64(n)
		}
	}()

	// Receive: receive the SSH_FXP_DATA from each read.
	// We only need the one workCh listener, though.
	// All result channels are len==1 buffered, so we can process them sequentially no problem.
	go func() {
		defer close(errCh)

		var resp sshfx.DataPacket

		for work := range workCh {
			// See readatFull for an explanation for why we use slices.Clip here.
			resp.Data = slices.Clip(work.b)

			n, err := f.cl.recvData(ctx, work.reqid, work.res, &resp)
			if n > len(work.b) {
				// We got an over-large packet, the Clip ensures this was a realloc.
				// So we have to copy it ourselves, but cannot use any of the extra data.
				n = copy(work.b, resp.Data)
			}

			if n < len(work.b) {
				// For normal disk files, it is guaranteed that this will read
				// the specified number of bytes, or up to end of file.
				// This implies, if we have a short read, that we have hit EOF.
				err = cmp.Or(err, io.EOF)
			}

			if err != nil {
				// Return the offset as the start + how much we read before the error.
				errCh <- rwErr{work.off + uint64(n), err}

				// DO NOT return.
				// We want to ensure that workCh is drained before wg.Wait returns.
			}
		}
	}()

	// Reduce: collect all the results into a relevant return: the earliest offset to return an error.
	var firstErr rwErr
	for rwErr := range errCh {
		if firstErr.err == nil || rwErr.off <= firstErr.off {
			firstErr = rwErr
		}

		// stop any more work from being distributed. (Just in case.)
		cancel()
	}

	if firstErr.err != nil {
		// firstErr.err != nil if and only if firstErr.off > our starting offset.
		return int(int64(firstErr.off) - off), f.wrapErr("readat", firstErr.err)
	}

	// As per spec for io.ReaderAt, we return nil error if and only if we read everything.
	return len(b), nil
}

// ReadAt reads len(b) bytes from the File starting at byte offset off.
// It returns the number of bytes read and the error, if any.
// ReadAt always returns a non-nil error when n < len(b).
// At the end of file, the error is io.EOF.
func (f *File) ReadAt(b []byte, off int64) (int, error) {
	if f == nil {
		return 0, fs.ErrInvalid
	}

	return f.readat(context.Background(), b, off)
}

// Read reads up to len(b) bytes from the File and stores them in b.
// It returns the number of bytes read and any error encountered.
// At end of file, Read returns 0, io.EOF.
func (f *File) Read(b []byte) (int, error) {
	if f == nil {
		return 0, fs.ErrInvalid
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	n, err := f.readat(context.Background(), b, f.offset)

	f.offset += int64(n)

	if errors.Is(err, io.EOF) && n != 0 {
		return n, nil
	}

	return n, err
}

func (f *File) writeToSequential(w io.Writer) (written int64, err error) {
	if f == nil {
		return 0, fs.ErrInvalid
	}

	handle, closed, err := f.handle.get()
	if err != nil {
		return 0, f.wrapErr("writeto", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	ctx := context.Background()
	b := make([]byte, f.cl.maxDataLen)

	req := &sshfx.ReadPacket{
		Handle: handle,
		Length: uint32(len(b)),
	}

	resp := sshfx.DataPacket{
		Data: b,
	}

	for {
		req.Offset = uint64(f.offset)

		read, err := f.cl.sendRead(ctx, closed, req, &resp)

		if read < 0 {
			panic("sftp: writeto: sendRead returned negative count")
		}

		if read > 0 {
			f.offset += int64(read)

			n, err := w.Write(b[:read])
			written += int64(n)

			if err != nil {
				return written, err
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return written, nil // return nil instead of EOF
			}

			return written, f.wrapErr("writeto", err)
		}
	}
}

// WriteTo writes the file to the given Writer.
// The return value is the number of bytes written, which may be different than the bytes read.
// Any error encountered during the write is also returned.
//
// This method is preferred over calling Read mulitple times
// to maximize throughput for transferring the entire file,
// especially over high latency links.
func (f *File) WriteTo(w io.Writer) (written int64, err error) {
	if f == nil {
		return 0, fs.ErrInvalid
	}

	handle, closed, err := f.handle.get()
	if err != nil {
		return 0, f.wrapErr("writeto", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	ctx := context.Background()

	chunkSize := f.cl.maxDataLen

	type work struct {
		reqid uint32
		res   chan result
		off   uint64
	}
	workCh := make(chan work, f.cl.maxInflight)

	// Once the writing Reduce phase has ended, all the feed work needs to unconditionally stop.
	sendCtx, cancel := context.WithCancel(ctx)

	defer func() {
		cancel() // Must be triggered before we drain!

		// Ensure we properly drain and discard to restore the result pool.
		for work := range workCh {
			f.cl.conn.discardBlocking(work.res)
		}
	}()

	var writeErr error

	// Dispatch: Dispatch into any number of Reads of length <= f.cl.maxDataLen.
	go func() {
		defer close(workCh)

		ctx := sendCtx // shadow ctx so we cannot accidentally reference the parent context here.

		req := &sshfx.ReadPacket{
			Handle: handle,
			Offset: uint64(f.offset),
			Length: uint32(chunkSize),
		}

		for {
			reqid, res, err := f.cl.conn.dispatch(closed, req)
			if err != nil {
				writeErr = err
				return
			}

			select {
			case workCh <- work{reqid, res, req.Offset}:
			case <-ctx.Done():
				// We're not sending this as work,
				// so we need to discard result and restore the result pool.
				f.cl.conn.discard(res)

				// Don't send the context error here.
				// We let the reduce code handle any parent context errors.
				return
			}

			req.Offset += uint64(chunkSize)
		}
	}()

	hint := f.cl.getDataBuf(chunkSize)

	// We want to return this data buffer back.
	// If we realloc from an over-long data packet, the Put() should ideally not let that in anyways.
	// So, better to return this specific buffer.
	defer f.cl.conn.bufPool.Put(hint)

	// one object and buffer to reduce allocs
	resp := sshfx.DataPacket{
		Data: hint,
	}

	// Reduce: receive the read request data, and write it out to the sink.
	// Since we issue them in order, the recv on the channel will be in order.
	for work := range workCh {
		n, recvErr := f.cl.recvData(ctx, work.reqid, work.res, &resp)
		// Because of how SFTP works, it should not be possible to return n > 0 && err != nil.
		// But we treat it like it could, just to keep consistency with the other Read+Write code.

		n = min(n, chunkSize) // Just in case we received an over-long data packet.

		// Because read requests are serialized,
		// this will always be the last successfully (and intentionally) read byte.
		f.offset = int64(work.off) + int64(n)

		if n > 0 {
			n, err := w.Write(resp.Data[:n])

			written += int64(n)

			if err != nil {
				return written, err // We don't want this err to get wrapped by the PathError below.
			}
		}

		if err := recvErr; err != nil {
			if errors.Is(err, io.EOF) {
				return written, nil // return nil instead of EOF
			}

			return written, f.wrapErr("writeto", err)
		}
	}

	return written, f.wrapErr("writeto", writeErr)
}

// WriteFile writes data to the named file, creating it if neccessary.
// If the file does not exist, WriteFile creates it with permissions perm (before umask);
// otherwise WriteFile truncates it before writing, without changing permissions.
// Since WriteFile requires multiple system calls to complete,
// a failure mid-operation can leave the file in a partially written state.
func (cl *Client) WriteFile(name string, data []byte, perm fs.FileMode) error {
	f, err := cl.OpenFile(name, OpenFlagWriteOnly|OpenFlagCreate|OpenFlagTruncate, perm)
	if err != nil {
		return err
	}

	_, err = f.Write(data)

	return cmp.Or(err, f.Close())
}

// ReadFile reads the named file and returns the contents.
// A successful call returns err == nil, not err == EOF.
// Because ReadFile reads the whole file, it does not treat an EOF from Read as an error to be reported.
//
// Note that ReadFile will call Stat on the file to get the file size,
// in order to avoid unnecessary allocations before reading in all the data.
// Some "read once" servers will delete the file if they recceive a stat call on an open file,
// and then the download will fail.
//
// TODO(puellannivis): Before release, we should resolve this, or have knobs to prevent it.
func (cl *Client) ReadFile(name string) ([]byte, error) {
	// TODO(puellanivis): we should use path.Split(), OpenDir() the parent, then use the FileInfo from readdir.
	// With rangedir, we could even save on collecting all of the name entries to then search through them.
	// This approach should work on read-once servers, even if the directory listing would be more expensive.
	// Maybe include an UseFstat(false) option again to trigger it?
	// There's a chance with case-insensitive servers, that Open(name) would work, but Glob(name) would not...
	// so, we might not be able to universally apply it as the default.

	f, err := cl.Open(name)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)

	// Don't trust the file size for pre-allocation unless it is a regular file.
	if fi, err := f.Stat(); err == nil && fi.Mode().IsRegular() {
		size := fi.Size()
		if int64(int(size)) == size {
			buf.Grow(int(size))
		}
	}

	_, err = f.WriteTo(buf)

	return buf.Bytes(), cmp.Or(err, f.Close())
}

// These aliases to the io package values are provided as a convenience to avoid needing two imports to use Seek.
const (
	SeekStart   = io.SeekStart   // seek relative to the origin of the file
	SeekCurrent = io.SeekCurrent // seek relative to the current offset
	SeekEnd     = io.SeekEnd     // seek relative to the end
)

// Seek sets the offset for the next Read or Write on file to offset,
// interpreted accoreding to whence:
// SeekStart means relative to the origin of the file,
// SeekCurrent means relative to the current offset,
// and SeekEnd means relative to the end.
// It returns the new offset and an error, if any.
//
// Note well, a whence of SeekEnd will make an SSH_FX_FSTAT request on the file handle.
// In some cases, this may mark a "mailbox"-style file as successfuly read,
// and the server will delete the file, and return an error for all later operations.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	if f == nil {
		return 0, fs.ErrInvalid
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	var abs int64
	switch whence {
	case SeekStart:
		abs = offset
	case SeekCurrent:
		abs = f.offset + offset
	case SeekEnd:
		fi, err := f.Stat()
		if err != nil {
			return 0, err
		}
		abs = fi.Size() + offset
	default:
		return 0, f.wrapErr("seek", fmt.Errorf("%w: invalid whence: %d", fs.ErrInvalid, whence))
	}

	if offset < 0 {
		return 0, f.wrapErr("seek", fmt.Errorf("%w: negative offset: %d", fs.ErrInvalid, whence))
	}

	f.offset = abs
	return abs, nil
}

// Sync commits the current contents of the file to stable storage.
// Typically, this means flushing the file system's in-memory copy of recently written data to disk.
//
// If the server did not announce support for the "fsync@openssh.com" extension,
// then no request will be sent,
// and Sync returns an *fs.PathError wrapping sshfx.StatusOpUnsupported.
func (f *File) Sync() error {
	if f == nil {
		return fs.ErrInvalid
	}

	handle, closed, err := f.handle.get()
	if err != nil {
		return f.wrapErr("fsync", err)
	}

	if !f.cl.hasExtension(openssh.ExtensionFSync()) {
		return f.wrapErr("fsync", sshfx.StatusOpUnsupported)
	}

	return f.wrapErr("fsync",
		f.cl.sendPacket(context.Background(), closed, &openssh.FSyncExtendedPacket{
			Handle: handle,
		}),
	)
}
