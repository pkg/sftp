package sftp

import (
	"encoding"
	"io"
	"os"
	"path"
	"sync"
	"time"

	"github.com/kr/fs"

	"golang.org/x/crypto/ssh"
)

const maxOutstandingPackets = 128 // default from openssh sftp client: 64

// New creates a new SFTP client on conn.
func NewClient(conn *ssh.Client) (*Client, error) {
	s, err := conn.NewSession()
	if err != nil {
		return nil, err
	}
	if err := s.RequestSubsystem("sftp"); err != nil {
		return nil, err
	}
	pw, err := s.StdinPipe()
	if err != nil {
		return nil, err
	}
	pr, err := s.StdoutPipe()
	if err != nil {
		return nil, err
	}

	return NewClientPipe(pr, pw)
}

// NewClientPipe creates a new SFTP client given a Reader and a WriteCloser.
// This can be used for connecting to an SFTP server over TCP/TLS or by using
// the system's ssh client program (e.g. via exec.Command).
func NewClientPipe(rd io.Reader, wr io.WriteCloser) (*Client, error) {
	sftp := &Client{
		w:         wr,
		r:         rd,
		done:      make(chan struct{}),
		pktsOutCh: make(chan idCh, maxOutstandingPackets),
	}
	if err := sftp.sendInit(); err != nil {
		return nil, err
	}

	if err := sftp.recvVersion(); err != nil {
		return nil, err
	}

	// start packet receiving goroutine
	ch := make(chan pkt, maxOutstandingPackets)
	go sftp.demux(ch)
	go sftp.mux(ch)

	return sftp, nil
}

// Client represents an SFTP session on a *ssh.ClientConn SSH connection.
// Multiple Clients can be active on a single SSH connection, and a Client
// may be called concurrently from multiple Goroutines.
//
// Client implements the github.com/kr/fs.FileSystem interface.
type Client struct {
	w      io.WriteCloser
	r      io.Reader
	idMu   sync.Mutex // locks mu and seralises access to nextid
	nextid uint32

	sendMu    sync.Mutex // guarantees atomic operation for sending packets
	pktsOutCh chan idCh
	done      chan struct{}
	closeOnce sync.Once
}

// pkt holds a packet to be sent or received from the server, including a potential send/receive error.
type pkt struct {
	typ  uint8
	data []uint8
	err  error
}

// idCh tracks a packet that is to be sent. The response packet is sent to the channel ch.
type idCh struct {
	id uint32
	ch chan<- pkt
	p  encoding.BinaryMarshaler
}

// Close closes the SFTP session.
func (c *Client) Close() error {
	// send exit requests to goroutines
	c.closeOnce.Do(func() { close(c.done) })
	return c.w.Close()
}

// Create creates the named file mode 0666 (before umask), truncating it if
// it already exists. If successful, methods on the returned File can be
// used for I/O; the associated file descriptor has mode O_RDWR.
func (c *Client) Create(path string) (*File, error) {
	return c.open(path, flags(os.O_RDWR|os.O_CREATE|os.O_TRUNC))
}

const sftpProtocolVersion = 3 // http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02

// demux receives packets from the server and sends them out to the
// appropiate channel. When an error is received, it is sent to the channel and
// no more packets are received.
func (c *Client) demux(outch chan<- pkt) {
	for {
		// receive packet
		typ, data, err := recvPacket(c.r)
		if err == io.EOF {
			return
		}

		p := pkt{typ, data, err}

		select {
		case outch <- p:
		case <-c.done:
			return
		}

		// stop receiving packets on error
		if err != nil {
			return
		}
	}
}

// mux processes packets from the channel and sends them out to the
// appropiate channel.
func (c *Client) mux(inch <-chan pkt) {
	chans := make(map[uint32]chan<- pkt)
	for {
		select {
		case <-c.done:
			return

		case idch := <-c.pktsOutCh:
			err := sendPacket(c.w, idch.p)
			if err != nil {
				idch.ch <- pkt{err: err}
			}
			chans[idch.id] = idch.ch

		case p := <-inch:
			id, _ := unmarshalUint32(p.data)

			// send packet to appropiate channel, otherwise silently drop it
			if ch, ok := chans[id]; ok {
				// send packet to process
				ch <- pkt{p.typ, p.data, p.err}

				// remove chan from map
				delete(chans, id)
			}
		}
	}
}

// sendPackets sends packet p, the response is delivered to channel ch.
func (c *Client) sendPacket(p encoding.BinaryMarshaler, id uint32, ch chan<- pkt) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	c.pktsOutCh <- idCh{ch: ch, id: id, p: p}

	return nil
}

func (c *Client) sendInit() error {
	return sendPacket(c.w, sshFxInitPacket{
		Version: sftpProtocolVersion, // http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02
	})
}

// returns the current value of c.nextid and increments. Can be called by
// multiple goroutines in parallel.
func (c *Client) nextId() uint32 {
	c.idMu.Lock()
	defer c.idMu.Unlock()
	v := c.nextid
	c.nextid++
	return v
}

func (c *Client) recvVersion() error {
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return err
	}
	if typ != ssh_FXP_VERSION {
		return &unexpectedPacketErr{ssh_FXP_VERSION, typ}
	}

	version, _ := unmarshalUint32(data)
	if version != sftpProtocolVersion {
		return &unexpectedVersionErr{sftpProtocolVersion, version}
	}

	return nil
}

// Walk returns a new Walker rooted at root.
func (c *Client) Walk(root string) *fs.Walker {
	return fs.WalkFS(root, c)
}

// ReadDir reads the directory named by dirname and returns a list of
// directory entries.
func (c *Client) ReadDir(p string) ([]os.FileInfo, error) {
	handle, err := c.opendir(p)
	if err != nil {
		return nil, err
	}
	defer c.close(handle) // this has to defer earlier than the lock below
	var attrs []os.FileInfo
	var done = false
	// TODO(fd0) send sshFxpReaddirPackets in parallel
	for !done {
		id := c.nextId()
		pkt, err1 := c.sendRequest(sshFxpReaddirPacket{
			Id:     id,
			Handle: handle,
		}, id)
		if err1 != nil {
			err = err1
			done = true
			break
		}
		switch pkt.typ {
		case ssh_FXP_NAME:
			sid, data := unmarshalUint32(pkt.data)
			if sid != id {
				return nil, &unexpectedIdErr{id, sid}
			}
			count, data := unmarshalUint32(data)
			for i := uint32(0); i < count; i++ {
				var filename string
				filename, data = unmarshalString(data)
				_, data = unmarshalString(data) // discard longname
				var attr *FileStat
				attr, data = unmarshalAttrs(data)
				if filename == "." || filename == ".." {
					continue
				}
				attrs = append(attrs, fileInfoFromStat(attr, path.Base(filename)))
			}
		case ssh_FXP_STATUS:
			// TODO(dfc) scope warning!
			err = eofOrErr(unmarshalStatus(id, pkt.data))
			done = true
		default:
			return nil, unimplementedPacketErr(pkt.typ)
		}
	}
	if err == io.EOF {
		err = nil
	}
	return attrs, err
}

func (c *Client) opendir(path string) (string, error) {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpOpendirPacket{
		Id:   id,
		Path: path,
	}, id)
	if err != nil {
		return "", err
	}
	switch pkt.typ {
	case ssh_FXP_HANDLE:
		sid, data := unmarshalUint32(pkt.data)
		if sid != id {
			return "", &unexpectedIdErr{id, sid}
		}
		handle, _ := unmarshalString(data)
		return handle, nil
	case ssh_FXP_STATUS:
		return "", unmarshalStatus(id, pkt.data)
	default:
		return "", unimplementedPacketErr(pkt.typ)
	}
}

func (c *Client) Lstat(p string) (os.FileInfo, error) {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpLstatPacket{
		Id:   id,
		Path: p,
	}, id)
	if err != nil {
		return nil, err
	}
	switch pkt.typ {
	case ssh_FXP_ATTRS:
		sid, data := unmarshalUint32(pkt.data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		attr, _ := unmarshalAttrs(data)
		return fileInfoFromStat(attr, path.Base(p)), nil
	case ssh_FXP_STATUS:
		return nil, unmarshalStatus(id, pkt.data)
	default:
		return nil, unimplementedPacketErr(pkt.typ)
	}
}

// ReadLink reads the target of a symbolic link.
func (c *Client) ReadLink(p string) (string, error) {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpReadlinkPacket{
		Id:   id,
		Path: p,
	}, id)
	if err != nil {
		return "", err
	}
	switch pkt.typ {
	case ssh_FXP_NAME:
		sid, data := unmarshalUint32(pkt.data)
		if sid != id {
			return "", &unexpectedIdErr{id, sid}
		}
		count, data := unmarshalUint32(data)
		if count != 1 {
			return "", unexpectedCount(1, count)
		}
		filename, _ := unmarshalString(data) // ignore dummy attributes
		return filename, nil
	case ssh_FXP_STATUS:
		return "", unmarshalStatus(id, pkt.data)
	default:
		return "", unimplementedPacketErr(pkt.typ)
	}
}

// setstat is a convience wrapper to allow for changing of various parts of the file descriptor.
func (c *Client) setstat(path string, flags uint32, attrs interface{}) error {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpSetstatPacket{
		Id:    id,
		Path:  path,
		Flags: flags,
		Attrs: attrs,
	}, id)
	if err != nil {
		return err
	}
	switch pkt.typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, pkt.data))
	default:
		return unimplementedPacketErr(pkt.typ)
	}
}

// Chtimes changes the access and modification times of the named file.
func (c *Client) Chtimes(path string, atime time.Time, mtime time.Time) error {
	type times struct {
		Atime uint32
		Mtime uint32
	}
	attrs := times{uint32(atime.Unix()), uint32(mtime.Unix())}
	return c.setstat(path, ssh_FILEXFER_ATTR_ACMODTIME, attrs)
}

// Chown changes the user and group owners of the named file.
func (c *Client) Chown(path string, uid, gid int) error {
	type owner struct {
		Uid uint32
		Gid uint32
	}
	attrs := owner{uint32(uid), uint32(gid)}
	return c.setstat(path, ssh_FILEXFER_ATTR_UIDGID, attrs)
}

// Chmod changes the permissions of the named file.
func (c *Client) Chmod(path string, mode os.FileMode) error {
	return c.setstat(path, ssh_FILEXFER_ATTR_PERMISSIONS, uint32(mode))
}

// Truncate sets the size of the named file. Although it may be safely assumed
// that if the size is less than its current size it will be truncated to fit,
// the SFTP protocol does not specify what behavior the server should do when setting
// size greater than the current size.
func (c *Client) Truncate(path string, size int64) error {
	return c.setstat(path, ssh_FILEXFER_ATTR_SIZE, uint64(size))
}

// Open opens the named file for reading. If successful, methods on the
// returned file can be used for reading; the associated file descriptor
// has mode O_RDONLY.
func (c *Client) Open(path string) (*File, error) {
	return c.open(path, flags(os.O_RDONLY))
}

// OpenFile is the generalized open call; most users will use Open or
// Create instead. It opens the named file with specified flag (O_RDONLY
// etc.). If successful, methods on the returned File can be used for I/O.
func (c *Client) OpenFile(path string, f int) (*File, error) {
	return c.open(path, flags(f))
}

func (c *Client) open(path string, pflags uint32) (*File, error) {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpOpenPacket{
		Id:     id,
		Path:   path,
		Pflags: pflags,
	}, id)
	if err != nil {
		return nil, err
	}
	switch pkt.typ {
	case ssh_FXP_HANDLE:
		sid, data := unmarshalUint32(pkt.data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		handle, _ := unmarshalString(data)
		return &File{c: c, path: path, handle: handle}, nil
	case ssh_FXP_STATUS:
		return nil, unmarshalStatus(id, pkt.data)
	default:
		return nil, unimplementedPacketErr(pkt.typ)
	}
}

// readAt sends a packet to read len(buf) bytes from the remote file indicated
// by handle starting from offset, the response packet is sent to ch. Returned
// is the packet id.
func (c *Client) readAt(handle string, ch chan<- pkt, offset uint64, buf []byte) (uint32, error) {
	id := c.nextId()
	err := c.sendPacket(sshFxpReadPacket{
		Id:     id,
		Handle: handle,
		Offset: offset,
		Len:    uint32(len(buf)),
	}, id, ch)
	if err != nil {
		return 0, err
	}

	return id, nil
}

// close closes a handle handle previously returned in the response
// to SSH_FXP_OPEN or SSH_FXP_OPENDIR. The handle becomes invalid
// immediately after this request has been sent.
func (c *Client) close(handle string) error {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpClosePacket{
		Id:     id,
		Handle: handle,
	}, id)
	if err != nil {
		return err
	}
	switch pkt.typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, pkt.data))
	default:
		return unimplementedPacketErr(pkt.typ)
	}
}

func (c *Client) fstat(handle string) (*FileStat, error) {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpFstatPacket{
		Id:     id,
		Handle: handle,
	}, id)
	if err != nil {
		return nil, err
	}
	switch pkt.typ {
	case ssh_FXP_ATTRS:
		sid, data := unmarshalUint32(pkt.data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		attr, _ := unmarshalAttrs(data)
		return attr, nil
	case ssh_FXP_STATUS:
		return nil, unmarshalStatus(id, pkt.data)
	default:
		return nil, unimplementedPacketErr(pkt.typ)
	}
}

// Join joins any number of path elements into a single path, adding a
// separating slash if necessary. The result is Cleaned; in particular, all
// empty strings are ignored.
func (c *Client) Join(elem ...string) string { return path.Join(elem...) }

// Remove removes the specified file or directory. An error will be returned if no
// file or directory with the specified path exists, or if the specified directory
// is not empty.
func (c *Client) Remove(path string) error {
	err := c.removeFile(path)
	if status, ok := err.(*StatusError); ok && status.Code == ssh_FX_FAILURE {
		err = c.removeDirectory(path)
	}
	return err
}

func (c *Client) removeFile(path string) error {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpRemovePacket{
		Id:       id,
		Filename: path,
	}, id)
	if err != nil {
		return err
	}
	switch pkt.typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, pkt.data))
	default:
		return unimplementedPacketErr(pkt.typ)
	}
}

func (c *Client) removeDirectory(path string) error {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpRmdirPacket{
		Id:   id,
		Path: path,
	}, id)
	if err != nil {
		return err
	}
	switch pkt.typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, pkt.data))
	default:
		return unimplementedPacketErr(pkt.typ)
	}
}

// Rename renames a file.
func (c *Client) Rename(oldname, newname string) error {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpRenamePacket{
		Id:      id,
		Oldpath: oldname,
		Newpath: newname,
	}, id)
	if err != nil {
		return err
	}
	switch pkt.typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, pkt.data))
	default:
		return unimplementedPacketErr(pkt.typ)
	}
}

// sendRequest sends a packet and blocks until the answer has arrived.
func (c *Client) sendRequest(p encoding.BinaryMarshaler, id uint32) (*pkt, error) {
	ch := make(chan pkt, 1)
	err := c.sendPacket(p, id, ch)
	if err != nil {
		return nil, err
	}

	// wait for response
	pkt := <-ch

	return &pkt, nil
}

// writeAt writes len(buf) bytes to the remote file indicated by handle starting
// from offset. The response packets are sent to channel ch, returned is the packet
// id or the sending error, if any.
func (c *Client) writeAt(handle string, ch chan<- pkt, offset uint64, buf []byte) (uint32, error) {
	id := c.nextId()
	err := c.sendPacket(sshFxpWritePacket{
		Id:     id,
		Handle: handle,
		Offset: offset,
		Length: uint32(len(buf)),
		Data:   buf,
	}, id, ch)

	return id, err
}

// Creates the specified directory. An error will be returned if a file or
// directory with the specified path already exists, or if the directory's
// parent folder does not exist (the method cannot create complete paths).
func (c *Client) Mkdir(path string) error {
	id := c.nextId()
	pkt, err := c.sendRequest(sshFxpMkdirPacket{
		Id:   id,
		Path: path,
	}, id)
	if err != nil {
		return err
	}
	switch pkt.typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, pkt.data))
	default:
		return unimplementedPacketErr(pkt.typ)
	}
}

// File represents a remote file.
type File struct {
	c      *Client
	path   string
	handle string
	offset uint64 // current offset within remote file
}

// Close closes the File, rendering it unusable for I/O. It returns an
// error, if any.
func (f *File) Close() error {
	return f.c.close(f.handle)
}

// Read reads up to len(b) bytes from the File. It returns the number of
// bytes read and an error, if any. EOF is signaled by a zero count with
// err set to io.EOF.
func (f *File) Read(b []byte) (int, error) {
	var firstErr error
	inFlight := 0
	read := 0
	offset := f.offset
	maxInFlight := 2 // for a slow start

	// only allocate channels with buffer size maxOutstandingPackets if buffer is sufficiently large
	maxPkts := min(len(b)/maxWritePacket+1, maxOutstandingPackets)

	// create channel for the response packets
	ch := make(chan pkt, maxPkts)

	type ack struct {
		id   uint32
		size int
		seen bool
		data []uint8
	}
	acks := make([]ack, 0, maxPkts)

	for inFlight > 0 || len(b) > 0 {
		for inFlight < maxInFlight && len(b) > 0 && firstErr == nil {
			// send packet
			l := min(len(b), maxWritePacket)
			id, err := f.c.readAt(f.handle, ch, offset, b[:l])

			// if an error occurred while sending, set firstErr and exit the sending loop
			if err != nil {
				firstErr = err
				break
			}

			offset += uint64(l)

			acks = append(acks, ack{id: id, size: l, data: b[:l]})
			b = b[l:]
			inFlight++
		}

		// if there are no packets in flight any more (e.g. because firstErr is set), exit the loop
		if inFlight == 0 {
			break
		}

		// otherwise try to process a response packet
	receiveLoop:
		select {
		case pkt := <-ch:
			inFlight--

			// extract id
			id, data := unmarshalUint32(pkt.data)

			// check for correct status packet
			if pkt.typ == ssh_FXP_STATUS {
				firstErr = eofOrErr(unmarshalStatus(id, pkt.data))
			} else if pkt.typ != ssh_FXP_DATA {
				firstErr = unimplementedPacketErr(pkt.typ)
			}

			if firstErr != nil {
				break receiveLoop
			}

			// here pkt.typ == ssh_FXP_DATA and firstErr == nil holds

			if maxInFlight < maxOutstandingPackets {
				maxInFlight = min(maxInFlight*2, maxOutstandingPackets)
			}

			// search id
			found := false
			idx := 0
			for i := 0; i < len(acks); i++ {
				if acks[i].id == id {
					idx = i
					found = true
				}
			}

			if found {
				acks[idx].seen = true
				l, data := unmarshalUint32(data)
				copy(acks[idx].data, data[:l])
				acks[idx].size = int(l)

				for len(acks) > 0 && acks[0].seen {
					f.offset += uint64(acks[0].size)
					read += acks[0].size

					if len(acks) > 1 {
						acks = acks[1:]
					} else {
						acks = []ack{}
					}
				}
			}
		}

	}
	return read, firstErr
}

// Stat returns the FileInfo structure describing file. If there is an
// error.
func (f *File) Stat() (os.FileInfo, error) {
	fs, err := f.c.fstat(f.handle)
	if err != nil {
		return nil, err
	}
	return fileInfoFromStat(fs, path.Base(f.path)), nil
}

// clamp writes to less than 32k
const maxWritePacket = 1 << 15

// Write writes len(b) bytes to the File. It returns the number of bytes
// written and an error, if any. Write returns a non-nil error when n !=
// len(b).
func (f *File) Write(b []byte) (int, error) {
	var (
		firstErr error
		inFlight int
		written  int
	)
	offset := f.offset

	// only allocate channels with buffer size maxOutstandingPackets if buffer is sufficiently large
	maxPkts := min(len(b)/maxWritePacket+1, maxOutstandingPackets)

	// create channel for the response packets
	ch := make(chan pkt, maxPkts)

	type ack struct {
		id   uint32
		size int
		seen bool
	}
	acks := make([]ack, 0, maxPkts)

	for inFlight > 0 || len(b) > 0 {
		for inFlight < maxPkts && len(b) > 0 && firstErr == nil {
			// send packet
			l := min(len(b), maxWritePacket)
			id, err := f.c.writeAt(f.handle, ch, offset, b[:l])

			// if an error occurred while sending, set firstErr and exit the sending loop
			if err != nil {
				firstErr = err
				break
			}

			offset += uint64(l)

			acks = append(acks, ack{id: id, size: l})
			b = b[l:]
			inFlight++
		}

		// if there are no packets in flight any more (e.g. because firstErr is set), exit the loop
		if inFlight == 0 {
			break
		}

		// otherwise try to process a response packet
	receiveLoop:
		select {
		case pkt := <-ch:
			inFlight--

			// extract id
			id, _ := unmarshalUint32(pkt.data)

			// check for correct status packet
			switch pkt.typ {
			case ssh_FXP_STATUS:
				if err := okOrErr(unmarshalStatus(id, pkt.data)); err != nil {
					if firstErr == nil {
						firstErr = err
					}
				}
			default:
				if firstErr == nil {
					firstErr = unimplementedPacketErr(pkt.typ)
				}
			}

			if firstErr != nil {
				break receiveLoop
			}

			// search id
			found := false
			idx := 0
			for i := 0; i < len(acks); i++ {
				if acks[i].id == id {
					idx = i
					found = true
				}
			}

			if found {
				acks[idx].seen = true

				for len(acks) > 0 && acks[0].seen {
					f.offset += uint64(acks[0].size)
					written += acks[0].size

					if len(acks) > 1 {
						acks = acks[1:]
					} else {
						acks = []ack{}
					}
				}
			}
		}
	}
	return written, firstErr
}

// Seek implements io.Seeker by setting the client offset for the next Read or
// Write. It returns the next offset read. Seeking before or after the end of
// the file is undefined. Seeking relative to the end calls Stat.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case os.SEEK_SET:
		f.offset = uint64(offset)
	case os.SEEK_CUR:
		f.offset = uint64(int64(f.offset) + offset)
	case os.SEEK_END:
		fi, err := f.Stat()
		if err != nil {
			return int64(f.offset), err
		}
		f.offset = uint64(fi.Size() + offset)
	default:
		return int64(f.offset), unimplementedSeekWhence(whence)
	}
	return int64(f.offset), nil
}

// Chown changes the uid/gid of the current file.
func (f *File) Chown(uid, gid int) error {
	return f.c.Chown(f.path, uid, gid)
}

// Chmod changes the permissions of the current file.
func (f *File) Chmod(mode os.FileMode) error {
	return f.c.Chmod(f.path, mode)
}

// Truncate sets the size of the current file. Although it may be safely assumed
// that if the size is less than its current size it will be truncated to fit,
// the SFTP protocol does not specify what behavior the server should do when setting
// size greater than the current size.
func (f *File) Truncate(size int64) error {
	return f.c.Truncate(f.path, size)
}

func min(a, b int) int {
	if a > b {
		return b
	}
	return a
}

// okOrErr returns nil if Err.Code is SSH_FX_OK, otherwise it returns the error.
func okOrErr(err error) error {
	if err, ok := err.(*StatusError); ok && err.Code == ssh_FX_OK {
		return nil
	}
	return err
}

func eofOrErr(err error) error {
	if err, ok := err.(*StatusError); ok && err.Code == ssh_FX_EOF {
		return io.EOF
	}
	return err
}

func unmarshalStatus(id uint32, data []byte) error {
	sid, data := unmarshalUint32(data)
	if sid != id {
		return &unexpectedIdErr{id, sid}
	}
	code, data := unmarshalUint32(data)
	msg, data := unmarshalString(data)
	lang, _ := unmarshalString(data)
	return &StatusError{
		Code: code,
		msg:  msg,
		lang: lang,
	}
}

// flags converts the flags passed to OpenFile into ssh flags.
// Unsupported flags are ignored.
func flags(f int) uint32 {
	var out uint32
	switch f & os.O_WRONLY {
	case os.O_WRONLY:
		out |= ssh_FXF_WRITE
	case os.O_RDONLY:
		out |= ssh_FXF_READ
	}
	if f&os.O_RDWR == os.O_RDWR {
		out |= ssh_FXF_READ | ssh_FXF_WRITE
	}
	if f&os.O_APPEND == os.O_APPEND {
		out |= ssh_FXF_APPEND
	}
	if f&os.O_CREATE == os.O_CREATE {
		out |= ssh_FXF_CREAT
	}
	if f&os.O_TRUNC == os.O_TRUNC {
		out |= ssh_FXF_TRUNC
	}
	if f&os.O_EXCL == os.O_EXCL {
		out |= ssh_FXF_EXCL
	}
	return out
}
