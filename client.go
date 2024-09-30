package sftp

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
	"github.com/pkg/sftp/v2/internal/pool"

	"golang.org/x/crypto/ssh"
)

type result struct {
	pkt *sshfx.RawPacket
	err error
}

type clientConn struct {
	reqid atomic.Uint32
	rd    io.Reader

	resPool *pool.WorkPool[result]

	bufPool *pool.SlicePool[[]byte, byte]
	pktPool *pool.Pool[sshfx.RawPacket]

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

func (c *clientConn) dispatch(req sshfx.PacketMarshaller) (uint32, chan result, error) {
	reqid := c.reqid.Add(1)

	header, payload, err := req.MarshalPacket(reqid, c.bufPool.Get())
	if err != nil {
		return reqid, nil, err
	}
	defer c.bufPool.Put(header)

	ch, ok := c.resPool.Get()
	if !ok {
		return reqid, nil, sshfx.StatusConnectionLost
	}

	c.mu.Lock()
	defer c.mu.Unlock()

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

func (c *clientConn) send(ctx context.Context, req sshfx.PacketMarshaller) (*sshfx.RawPacket, error) {
	reqid, ch, err := c.dispatch(req)
	if err != nil {
		return nil, err
	}

	return c.recv(ctx, reqid, ch)
}

type ClientOption func(*Client)

type Client struct {
	conn clientConn

	maxPacket   uint32
	maxDataLen  int
	maxInflight int

	exts map[string]string
}

func getPacket[PKT any, P interface {
	sshfx.Packet
	*PKT
}](ctx context.Context, cl *Client, req sshfx.PacketMarshaller) (*PKT, error) {
	raw, err := cl.conn.send(ctx, req)
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

func (cl *Client) sendPacket(ctx context.Context, req sshfx.PacketMarshaller) error {
	reqid, ch, err := cl.conn.dispatch(req)
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

func (cl *Client) sendRead(ctx context.Context, req *sshfx.ReadPacket, resp *sshfx.DataPacket) (int, error) {
	reqid, ch, err := cl.conn.dispatch(req)
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
func NewClient(ctx context.Context, conn *ssh.Client) (*Client, error) {
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

	return NewClientPipe(ctx, r, w)
}

// NewClientPipe attempts to negotiate an SFTP session with the given read and write channels.
//
// The given context is only used for the negotiation of init and version packets.
func NewClientPipe(ctx context.Context, rd io.Reader, wr io.WriteCloser) (*Client, error) {
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

	exts, err := cl.conn.handshake(ctx, cl.maxPacket)
	if err != nil {
		return nil, err
	}

	cl.exts = exts

	cl.conn.resPool = pool.NewWorkPool[result](cl.maxInflight)

	cl.conn.bufPool = pool.NewSlicePool[[]byte](cl.maxInflight, int(cl.maxPacket))
	cl.conn.pktPool = pool.NewPool[sshfx.RawPacket](cl.maxInflight)

	go func() {
		if err := cl.conn.recvLoop(cl.maxPacket); err != nil {
			cl.conn.disconnect(err)
		}
	}()

	return cl, nil
}

func (cl *Client) ReportMetrics(wr io.Writer) {
	if cl.conn.bufPool != nil {
		hits, total := cl.conn.bufPool.Hits()

		fmt.Printf("bufpool hit rate: %d / %d = %f\n", hits, total, float64(hits)/float64(total))
	}
}

func (cl *Client) Close() error {
	cl.conn.disconnect(nil)
	cl.conn.wr.Close()
	return nil
}

func (cl *Client) Mkdir(name string, perm fs.FileMode) error {
	err := cl.sendPacket(context.Background(), &sshfx.MkdirPacket{
		Path: name,
		Attrs: sshfx.Attributes{
			Flags:       sshfx.AttrPermissions,
			Permissions: sshfx.FileMode(perm.Perm()),
		},
	})
	if err != nil {
		return &fs.PathError{Op: "mkdir", Path: name, Err: err}
	}

	return nil
}

func (cl *Client) MkdirAll(name string, perm fs.FileMode) error {
	// Fast path: if we can tell whether name is a directory or file, stop with success or error.
	dir, err := cl.Stat(name)
	if err == nil {
		if dir.IsDir() {
			return nil
		}

		return &fs.PathError{Op: "mkdir", Path: name, Err: syscall.ENOTDIR}
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

func (cl *Client) Remove(name string) error {
	ctx := context.Background()

	err := cl.sendPacket(ctx, &sshfx.RemovePacket{
		Path: name,
	})
	if err == nil {
		return nil
	}

	err1 := cl.sendPacket(ctx, &sshfx.RmdirPacket{
		Path: name,
	})
	if err1 == nil {
		return nil
	}

	// Both failed: figure out which error to return.
	if err != err1 {
		attrs, err2 := getPacket[sshfx.AttrsPacket](ctx, cl, &sshfx.StatPacket{
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

	return &fs.PathError{Op: "remove", Path: name, Err: err}
}

func (cl *Client) setstat(ctx context.Context, name string, attrs *sshfx.Attributes) error {
	err := cl.sendPacket(ctx, &sshfx.SetStatPacket{
		Path:  name,
		Attrs: *attrs,
	})
	if err != nil {
		return &fs.PathError{Op: "setstat", Path: name, Err: err}
	}

	return nil
}

func (cl *Client) Truncate(name string, size int64) error {
	return cl.setstat(context.Background(), name, &sshfx.Attributes{
		Flags: sshfx.AttrSize,
		Size:  uint64(size),
	})
}

func (cl *Client) Chmod(name string, mode fs.FileMode) error {
	return cl.setstat(context.Background(), name, &sshfx.Attributes{
		Flags:       sshfx.AttrPermissions,
		Permissions: sshfx.FromGoFileMode(mode),
	})
}

func (cl *Client) Chown(name string, uid, gid int) error {
	return cl.setstat(context.Background(), name, &sshfx.Attributes{
		Flags: sshfx.AttrUIDGID,
		UID:   uint32(uid),
		GID:   uint32(gid),
	})
}

func (cl *Client) Chtimes(name string, atime, mtime time.Time) error {
	return cl.setstat(context.Background(), name, &sshfx.Attributes{
		Flags: sshfx.AttrACModTime,
		ATime: uint32(atime.Unix()),
		MTime: uint32(mtime.Unix()),
	})
}

func (cl *Client) RealPath(name string) (string, error) {
	pkt, err := getPacket[sshfx.PathPseudoPacket](context.Background(), cl, &sshfx.RealPathPacket{
		Path: name,
	})
	if err != nil {
		return "", &fs.PathError{Op: "realpath", Path: name, Err: err}
	}

	return pkt.Path, nil
}

func (cl *Client) ReadLink(name string) (string, error) {
	pkt, err := getPacket[sshfx.PathPseudoPacket](context.Background(), cl, &sshfx.ReadLinkPacket{
		Path: name,
	})
	if err != nil {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: err}
	}

	return pkt.Path, nil
}

func (cl *Client) Rename(oldpath, newpath string) error {
	if cl.hasExtension(openssh.ExtensionPOSIXRename()) {
		err := cl.sendPacket(context.Background(), &openssh.POSIXRenameExtendedPacket{
			OldPath: oldpath,
			NewPath: newpath,
		})
		if err != nil {
			return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
		}

		return nil
	}

	err := cl.sendPacket(context.Background(), &sshfx.RenamePacket{
		OldPath: oldpath,
		NewPath: newpath,
	})
	if err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}

	return nil
}

func (cl *Client) Symlink(oldname, newname string) error {
	err := cl.sendPacket(context.Background(), &sshfx.SymlinkPacket{
		LinkPath:   newname,
		TargetPath: oldname,
	})
	if err != nil {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: err}
	}

	return nil
}

func (cl *Client) hasExtension(ext *sshfx.ExtensionPair) bool {
	return cl.exts[ext.Name] == ext.Data
}

func (cl *Client) Link(oldname, newname string) error {
	if !cl.hasExtension(openssh.ExtensionHardlink()) {
		return &os.LinkError{Op: "hardlink", Old: oldname, New: newname, Err: sshfx.StatusOPUnsupported}
	}

	err := cl.sendPacket(context.Background(), &openssh.HardlinkExtendedPacket{
		NewPath: newname,
		OldPath: oldname,
	})
	if err != nil {
		return &os.LinkError{Op: "hardlink", Old: oldname, New: newname, Err: err}
	}

	return nil
}

func (cl *Client) Readdir(name string) ([]fs.FileInfo, error) {
	d, err := cl.OpenDir(name)
	if err != nil {
		return nil, err
	}
	defer d.Close()

	return d.Readdir(0)
}

func (cl *Client) ReadDir(name string) ([]fs.DirEntry, error) {
	d, err := cl.OpenDir(name)
	if err != nil {
		return nil, err
	}
	defer d.Close()

	return d.ReadDir(0)
}

func (cl *Client) stat(name string) (*sshfx.NameEntry, error) {
	pkt, err := getPacket[sshfx.AttrsPacket](context.Background(), cl, &sshfx.StatPacket{
		Path: name,
	})
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}

	return &sshfx.NameEntry{
		Filename: name,
		Attrs:    pkt.Attrs,
	}, nil
}

func (cl *Client) Stat(name string) (fs.FileInfo, error) {
	return cl.stat(name)
}

func (cl *Client) LStat(name string) (fs.FileInfo, error) {
	pkt, err := getPacket[sshfx.AttrsPacket](context.Background(), cl, &sshfx.LStatPacket{
		Path: name,
	})
	if err != nil {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: err}
	}

	return &sshfx.NameEntry{
		Filename: name,
		Attrs:    pkt.Attrs,
	}, nil
}

type Dir struct {
	cl   *Client
	name string

	mu      sync.RWMutex
	handle  string
	entries []*sshfx.NameEntry
}

func (cl *Client) OpenDir(name string) (*Dir, error) {
	pkt, err := getPacket[sshfx.HandlePacket](context.Background(), cl, &sshfx.OpenDirPacket{
		Path: name,
	})
	if err != nil {
		return nil, &fs.PathError{Op: "opendir", Path: name, Err: err}
	}

	return &Dir{
		cl:     cl,
		name:   name,
		handle: pkt.Handle,
	}, nil
}

func (d *Dir) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handle == "" {
		return &fs.PathError{Op: "close", Path: d.name, Err: fs.ErrClosed}
	}

	// The design principle here is that when `openssh-portable/sftp-server.c` is doing `handle_close`,
	// it will unconditionally mark the handle as unused,
	// so we need to also unconditionally mark this handle as invalid.
	// By invalidating our local copy of the handle,
	// we ensure that there cannot be any erroneous use-after-close requests sent after Close.

	handle := d.handle
	d.handle = ""

	err := d.cl.sendPacket(context.Background(), &sshfx.ClosePacket{
		Handle: handle,
	})
	if err != nil {
		return &fs.PathError{Op: "close", Path: d.name, Err: err}
	}

	return nil
}

func (d *Dir) Name() string {
	return d.name
}

// readdir performs a single SSH_FXP_READDIR request.
// Callers must guarantee synchronization by either holding the file lock, or holding an exclusive reference.
func (d *Dir) readdir() ([]*sshfx.NameEntry, error) {
	pkt, err := getPacket[sshfx.NamePacket](context.Background(), d.cl, &sshfx.ReadDirPacket{
		Handle: d.handle,
	})
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: d.name, Err: err}
	}

	return pkt.Entries, nil
}

func (d *Dir) Readdir(n int) ([]fs.FileInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handle == "" {
		return nil, &fs.PathError{Op: "readdir", Path: d.name, Err: fs.ErrClosed}
	}

	var ret []fs.FileInfo

	// We have saved entries, use those to first try and satisfy the request.
	if len(d.entries) > 0 {
		for _, ent := range d.entries {
			if n > 0 && len(ret) >= n {
				break
			}

			ret = append(ret, ent)
		}

		remaining := copy(d.entries, d.entries[len(ret):])
		clear(d.entries[remaining:])
		d.entries = d.entries[:remaining]
	}

	for n <= 0 || len(ret) < n {
		entries, err := d.readdir()

		for i, ent := range entries {
			if n > 0 && len(ret) >= n {
				// copy entries into a new slice, to avoid aliasing and pinning the earlier entries.
				d.entries = append(d.entries, entries[i:]...)
				break
			}

			ret = append(ret, ent)
		}

		if err != nil {
			if len(ret) == 0 {
				return nil, err
			}

			return ret, nil
		}
	}

	return ret, nil
}

func (d *Dir) ReadDir(n int) ([]fs.DirEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.handle == "" {
		return nil, &fs.PathError{Op: "readdir", Path: d.name, Err: fs.ErrClosed}
	}

	var ret []fs.DirEntry

	// We have saved entries, use those to first try and satisfy the request.
	if len(d.entries) > 0 {
		for _, ent := range d.entries {
			if n > 0 && len(ret) >= n {
				break
			}

			ret = append(ret, ent)
		}

		remaining := copy(d.entries, d.entries[len(ret):])
		clear(d.entries[remaining:])
		d.entries = d.entries[:remaining]
	}

	for n <= 0 || len(ret) < n {
		entries, err := d.readdir()

		for i, ent := range entries {
			if n > 0 && len(ret) >= n {
				// copy entries into a new slice, to avoid aliasing and pinning the earlier entries.
				d.entries = append(d.entries, entries[i:]...)
				break
			}

			ret = append(ret, ent)
		}

		if err != nil {
			if len(ret) == 0 {
				return nil, err
			}

			return ret, nil
		}
	}

	return ret, nil
}

type File struct {
	cl   *Client
	name string

	mu     sync.RWMutex
	handle string
	offset int64 // current offset within remote file
}

// These aliases to the os package values are provided as a convenience to avoid needing two imports to use OpenFile.
const (
	// Exactly one of OpenReadOnly, OpenWriteOnly, OpenReadWrite must be specified.
	OpenFlagReadOnly  = os.O_RDONLY
	OpenFlagWriteOnly = os.O_WRONLY
	OpenFlagReadWrite = os.O_RDWR
	// The remaining values may be or’ed in to control behavior.
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

func (cl *Client) Open(name string) (*File, error) {
	return cl.OpenFile(name, OpenFlagReadOnly, 0)
}

func (cl *Client) Create(name string) (*File, error) {
	return cl.OpenFile(name, OpenFlagReadWrite|OpenFlagCreate|OpenFlagTruncate, 0666)
}

func (cl *Client) OpenFile(name string, flag int, perm fs.FileMode) (*File, error) {
	pkt, err := getPacket[sshfx.HandlePacket](context.Background(), cl, &sshfx.OpenPacket{
		Filename: name,
		PFlags:   toPortableFlags(flag),
		Attrs: sshfx.Attributes{
			Flags:       sshfx.AttrPermissions,
			Permissions: sshfx.FileMode(perm.Perm()),
		},
	})
	if err != nil {
		return nil, err
	}

	return &File{
		cl:     cl,
		name:   name,
		handle: pkt.Handle,
	}, nil
}

func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.handle == "" {
		return &fs.PathError{Op: "close", Path: f.name, Err: fs.ErrClosed}
	}

	// The design principle here is that when `openssh-portable/sftp-server.c` is doing `handle_close`,
	// it will unconditionally mark the handle as unused,
	// so we need to also unconditionally mark this handle as invalid.
	// By invalidating our local copy of the handle,
	// we ensure that there cannot be any erroneous use-after-close requests sent after Close.

	handle := f.handle
	f.handle = ""

	err := f.cl.sendPacket(context.Background(), &sshfx.ClosePacket{
		Handle: handle,
	})
	if err != nil {
		return &fs.PathError{Op: "close", Path: f.name, Err: err}
	}

	return nil
}

func (f *File) Name() string {
	return f.name
}

func (f *File) setstat(ctx context.Context, attrs *sshfx.Attributes) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.handle == "" {
		return &fs.PathError{Op: "fsetstat", Path: f.name, Err: fs.ErrClosed}
	}

	err := f.cl.sendPacket(ctx, &sshfx.FSetStatPacket{
		Handle: f.handle,
		Attrs:  *attrs,
	})
	if err != nil {
		return &fs.PathError{Op: "fsetstat", Path: f.name, Err: err}
	}

	return nil
}

func (f *File) Truncate(size int64) error {
	return f.setstat(context.Background(), &sshfx.Attributes{
		Flags: sshfx.AttrSize,
		Size:  uint64(size),
	})
}

func (f *File) Chmod(mode fs.FileMode) error {
	return f.setstat(context.Background(), &sshfx.Attributes{
		Flags:       sshfx.AttrPermissions,
		Permissions: sshfx.FromGoFileMode(mode),
	})
}

func (f *File) Chown(uid, gid int) error {
	return f.setstat(context.Background(), &sshfx.Attributes{
		Flags: sshfx.AttrUIDGID,
		UID:   uint32(uid),
		GID:   uint32(gid),
	})
}

func (f *File) Chtimes(atime, mtime time.Time) error {
	return f.setstat(context.Background(), &sshfx.Attributes{
		Flags: sshfx.AttrACModTime,
		ATime: uint32(atime.Unix()),
		MTime: uint32(mtime.Unix()),
	})
}

func (f *File) stat() (*sshfx.NameEntry, error) {
	pkt, err := getPacket[sshfx.AttrsPacket](context.Background(), f.cl, &sshfx.FStatPacket{
		Handle: f.handle,
	})
	if err != nil {
		return nil, &fs.PathError{Op: "fstat", Path: f.name, Err: err}
	}

	return &sshfx.NameEntry{
		Filename: f.name,
		Attrs:    pkt.Attrs,
	}, nil
}

func (f *File) Stat() (fs.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.handle == "" {
		return nil, &fs.PathError{Op: "fstat", Path: f.name, Err: fs.ErrClosed}
	}

	return f.stat()
}

func (f *File) writeAtFull(ctx context.Context, b []byte, off int64) (written int, err error) {
	req := &sshfx.WritePacket{
		Handle: f.handle,
		Offset: uint64(off),
	}

	chunkSize := f.cl.maxDataLen

	for len(b) > 0 {
		n := min(len(b), chunkSize)

		req.Data, b = b[:n], b[n:]

		err = f.cl.sendPacket(ctx, req)
		if err != nil {
			return written, &fs.PathError{Op: "writeat", Path: f.name, Err: err}
		}

		req.Offset += uint64(n)
		written += n
	}

	return written, nil
}

func (f *File) writeAt(ctx context.Context, b []byte, off int64) (written int, err error) {
	if f.handle == "" {
		return 0, &fs.PathError{Op: "writeat", Path: f.name, Err: fs.ErrClosed}
	}

	if len(b) <= f.cl.maxDataLen {
		// This should be able to be serviced with just 1 request.
		// So, just do it directly.
		return f.writeAtFull(ctx, b, off)
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
			Handle: f.handle,
			Offset: uint64(f.offset),
		}

		for len(b) > 0 {
			n := min(len(b), chunkSize)

			req.Data, b = b[:n], b[n:]

			reqid, res, err := f.cl.conn.dispatch(req)
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
		// Either way, this should be the last successfully write offset.
		written := int(int64(firstErr.off) - f.offset)
		f.offset = int64(firstErr.off)

		return written, firstErr.err
	}

	// We didn’t hit any errors, so we must have written all the bytes in the buffer.
	written = len(b)
	f.offset += int64(written)

	return written, nil
}

func (f *File) WriteAt(b []byte, off int64) (n int, err error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.writeAt(context.Background(), b, off)
}

func (f *File) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	n, err := f.writeAt(context.Background(), b, f.offset)
	f.offset += int64(n)

	return n, err
}

func (f *File) WriteString(s string) (n int, err error) {
	b := unsafe.Slice(unsafe.StringData(s), len(s))
	return f.Write(b)
}

func (f *File) readFromSequential(r io.Reader) (read int64, err error) {
	ctx := context.Background()
	b := make([]byte, f.cl.maxDataLen)

	req := &sshfx.WritePacket{
		Handle: f.handle,
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

			err1 := f.cl.sendPacket(ctx, req)
			if err1 == nil {
				// Only increment file offset, if we got a sucess back.
				f.offset += int64(n)
			}

			err = cmp.Or(err, err1)
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return read, nil // return nil explicitly
			}

			return read, err
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
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.handle == "" {
		return 0, fs.ErrClosed
	}

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
			Handle: f.handle,
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

				reqid, res, err1 := f.cl.conn.dispatch(req)
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
		return read, firstErr.err
	}

	// We didn’t hit any errors, so we must have written all the bytes that we read until EOF.
	f.offset += read
	return read, nil
}

// readAtFull attempts to read the whole entire length of the buffer from the file starting at the offset.
// It will continue progressively reading into the buffer until it fills the whole buffer, or an error occurs.
//
// This is prefered over io.ReadFull, because it can reuse read and data packet allocations.
func (f *File) readAtFull(ctx context.Context, b []byte, off int64) (read int, err error) {
	req := &sshfx.ReadPacket{
		Handle: f.handle,
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

		m, err := f.cl.sendRead(ctx, req, &resp)

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
			if errors.Is(err, io.EOF) {
				return read, io.EOF // io.Copy does not allow this to be wrapped.
			}

			return read, &fs.PathError{Op: "readat", Path: f.name, Err: err}
		}
	}

	return read, nil
}

func (f *File) readAt(ctx context.Context, b []byte, off int64) (read int, err error) {
	if f.handle == "" {
		return 0, &fs.PathError{Op: "readat", Path: f.name, Err: fs.ErrClosed}
	}

	if len(b) <= f.cl.maxDataLen {
		// This should be able to be serviced most times with only 1 request.
		// So, just do it sequentially.
		return f.readAtFull(ctx, b, off)
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
			Handle: f.handle,
			Offset: uint64(off),
		}

		for len(b) > 0 {
			n := min(len(b), chunkSize)

			req.Length = uint32(n)

			reqid, res, err := f.cl.conn.dispatch(req)
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
			// See readAtFull for an explanation for why we use slices.Clip here.
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
		return int(int64(firstErr.off) - off), firstErr.err
	}

	// As per spec for io.ReaderAt, we return nil error if and only if we read everything.
	return len(b), nil
}

func (f *File) ReadAt(b []byte, off int64) (int, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.readAt(context.Background(), b, off)
}

func (f *File) Read(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	n, err := f.readAt(context.Background(), b, f.offset)

	f.offset += int64(n)

	return n, err
}

func (f *File) writeToSequential(w io.Writer) (written int64, err error) {
	ctx := context.Background()
	b := make([]byte, f.cl.maxDataLen)

	req := &sshfx.ReadPacket{
		Handle: f.handle,
		Length: uint32(len(b)),
	}

	resp := sshfx.DataPacket{
		Data: b,
	}

	for {
		req.Offset = uint64(f.offset)

		read, err := f.cl.sendRead(ctx, req, &resp)

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
				return written, nil // return nil explicitly.
			}

			return written, &fs.PathError{Op: "readat", Path: f.name, Err: err}
		}
	}
}

// WriteTo writes the file to the given Writer.
// The return value is the number of bytes written.
// Any error encountered during the write is also returned.
//
// This method is preferred over calling Read mulitple times
// to maximize throughput for transferring the entire file,
// especially over high latency links.
func (f *File) WriteTo(w io.Writer) (written int64, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.handle == "" {
		return 0, &fs.PathError{Op: "writeto", Path: f.name, Err: fs.ErrClosed}
	}

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
			Handle: f.handle,
			Offset: uint64(f.offset),
			Length: uint32(chunkSize),
		}

		for {
			reqid, res, err := f.cl.conn.dispatch(req)
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
				return written, nil
			}

			return written, &fs.PathError{Op: "readat", Path: f.name, Err: err}
		}
	}

	return written, writeErr
}

func (cl *Client) WriteFile(name string, data []byte, perm fs.FileMode) error {
	f, err := cl.OpenFile(name, OpenFlagWriteOnly|OpenFlagCreate|OpenFlagTruncate, perm)
	if err != nil {
		return err
	}

	_, err = f.Write(data)

	return cmp.Or(err, f.Close())
}

func (cl *Client) ReadFile(name string) ([]byte, error) {
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

const (
	SeekStart   = io.SeekStart   // seek relative to the origin of the file
	SeekCurrent = io.SeekCurrent // seek relative to the current offset
	SeekEnd     = io.SeekEnd     // seek relative to the end
)

// Seek implements io.Seeker by setting the client offset for the next Read or
// Write. It returns the next offset read. Seeking before or after the end of
// the file is undefined. Seeking relative to the end calls Stat.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.handle == "" {
		return 0, &fs.PathError{Op: "seek", Path: f.name, Err: fs.ErrClosed}
	}

	switch whence {
	case SeekStart:
	case SeekCurrent:
		offset += f.offset
	case SeekEnd:
		fi, err := f.Stat()
		if err != nil {
			return f.offset, err
		}
		offset += fi.Size()
	default:
		return f.offset, &fs.PathError{
			Op:   "seek",
			Path: f.name,
			Err:  fmt.Errorf("%w: invalid whence: %d", fs.ErrInvalid, whence),
		}
	}

	if offset < 0 {
		return f.offset, &fs.PathError{
			Op:   "seek",
			Path: f.name,
			Err:  fmt.Errorf("%w: negative offset: %d", fs.ErrInvalid, offset),
		}
	}

	f.offset = offset
	return f.offset, nil
}

func (f *File) Sync() error {
	if !f.cl.hasExtension(openssh.ExtensionFSync()) {
		return &fs.PathError{Op: "fsync", Path: f.name, Err: sshfx.StatusOPUnsupported}
	}

	err := f.cl.sendPacket(context.Background(), &openssh.FSyncExtendedPacket{
		Handle: f.handle,
	})
	if err != nil {
		return &fs.PathError{Op: "fsync", Path: f.name, Err: err}
	}

	return nil
}
