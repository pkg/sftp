package sftp

import (
	"io"
	"os"
	"path"
	"sync"

	"github.com/davecheney/fs"

	"code.google.com/p/go.crypto/ssh"
)

// New creates a new SFTP client on conn.
func NewClient(conn *ssh.ClientConn) (*Client, error) {
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
	sftp := &Client{
		w: pw,
		r: pr,
	}
	if err := sftp.sendInit(); err != nil {
		return nil, err
	}
	return sftp, sftp.recvVersion()
}

// Client represents an SFTP session on a *ssh.ClientConn SSH connection.
// Multiple Clients can be active on a single SSH connection, and a Client
// may be called concurrently from multiple Goroutines.
type Client struct {
	w      io.WriteCloser
	r      io.Reader
	mu     sync.Mutex // locks mu and seralises commands to the server
	nextid uint32
}

// Close closes the SFTP session.
func (c *Client) Close() error { return c.w.Close() }

// Create creates the named file mode 0666 (before umask), truncating it if
// it already exists. If successful, methods on the returned File can be
// used for I/O; the associated file descriptor has mode O_RDWR.
func (c *Client) Create(path string) (*File, error) {
	return c.open(path, ssh_FXF_READ|ssh_FXF_WRITE|ssh_FXF_CREAT|ssh_FXF_TRUNC)
}

func (c *Client) sendInit() error {
	type packet struct {
		Type       byte
		Version    uint32
		Extensions []struct {
			Name, Data string
		}
	}
	return sendPacket(c.w, packet{
		Type:    ssh_FXP_INIT,
		Version: 3, // http://tools.ietf.org/html/draft-ietf-secsh-filexfer-02
	})
}

// returns the current value of c.nextid and increments it
// callers is expected to hold c.mu
func (c *Client) nextId() uint32 {
	v := c.nextid
	c.nextid++
	return v
}

func (c *Client) recvVersion() error {
	typ, _, err := recvPacket(c.r)
	if err != nil {
		return err
	}
	if typ != ssh_FXP_VERSION {
		return &unexpectedPacketErr{ssh_FXP_VERSION, typ}
	}
	return nil
}

// Walk returns a new Walker rooted at root.
func (c *Client) Walk(root string) *fs.Walker {
	return fs.WalkFunc(root, c.Lstat, c.readDir)
}

func (c *Client) readDir(p string) ([]os.FileInfo, error) {
	handle, err := c.opendir(p)
	if err != nil {
		return nil, err
	}
	var attrs []os.FileInfo
	c.mu.Lock()
	defer c.mu.Unlock()
	var done = false
	for !done {
		type packet struct {
			Type   byte
			Id     uint32
			Handle string
		}
		id := c.nextId()
		if err := sendPacket(c.w, packet{
			Type:   ssh_FXP_READDIR,
			Id:     id,
			Handle: handle,
		}); err != nil {
			return nil, err
		}
		typ, data, err := recvPacket(c.r)
		if err != nil {
			return nil, err
		}
		switch typ {
		case ssh_FXP_NAME:
			sid, data := unmarshalUint32(data)
			if sid != id {
				return nil, &unexpectedIdErr{id, sid}
			}
			count, data := unmarshalUint32(data)
			for i := uint32(0); i < count; i++ {
				var filename string
				filename, data = unmarshalString(data)
				_, data = unmarshalString(data) // discard longname
				var attr *attr
				attr, data = unmarshalAttrs(data)
				if filename == "." || filename == ".." {
					continue
				}
				attr.name = path.Base(filename)
				attrs = append(attrs, attr)
			}
		case ssh_FXP_STATUS:
			sid, data := unmarshalUint32(data)
			if sid != id {
				return nil, &unexpectedIdErr{id, sid}
			}
			code, data := unmarshalUint32(data)
			msg, data := unmarshalString(data)
			lang, _ := unmarshalString(data)
			err = &StatusError{
				Code: code,
				msg:  msg,
				lang: lang,
			}
			done = true
		default:
			return nil, unimplementedPacketErr(typ)
		}
	}
	// TODO(dfc) closedir
	return attrs, err
}
func (c *Client) opendir(path string) (string, error) {
	type packet struct {
		Type byte
		Id   uint32
		Path string
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type: ssh_FXP_OPENDIR,
		Id:   id,
		Path: path,
	}); err != nil {
		return "", err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return "", err
	}
	switch typ {
	case ssh_FXP_HANDLE:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return "", &unexpectedIdErr{id, sid}
		}
		handle, _ := unmarshalString(data)
		return handle, nil
	case ssh_FXP_STATUS:
		return "", unmarshalStatus(id, data)
	default:
		return "", unimplementedPacketErr(typ)
	}
}

func (c *Client) Lstat(p string) (os.FileInfo, error) {
	type packet struct {
		Type byte
		Id   uint32
		Path string
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type: ssh_FXP_LSTAT,
		Id:   id,
		Path: p,
	}); err != nil {
		return nil, err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return nil, err
	}
	switch typ {
	case ssh_FXP_ATTRS:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		attr, _ := unmarshalAttrs(data)
		attr.name = path.Base(p)
		return attr, nil
	case ssh_FXP_STATUS:
		return nil, unmarshalStatus(id, data)
	default:
		return nil, unimplementedPacketErr(typ)
	}
}

// Open opens the named file for reading. If successful, methods on the
// returned file can be used for reading; the associated file descriptor
// has mode O_RDONLY.
func (c *Client) Open(path string) (*File, error) {
	return c.open(path, ssh_FXF_READ)
}

func (c *Client) open(path string, pflags uint32) (*File, error) {
	type packet struct {
		Type   byte
		Id     uint32
		Path   string
		Pflags uint32
		Flags  uint32 // ignored
		Size   uint64 // ignored
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type:   ssh_FXP_OPEN,
		Id:     id,
		Path:   path,
		Pflags: pflags,
	}); err != nil {
		return nil, err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return nil, err
	}
	switch typ {
	case ssh_FXP_HANDLE:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		handle, _ := unmarshalString(data)
		return &File{c: c, path: path, handle: handle}, nil
	case ssh_FXP_STATUS:
		return nil, unmarshalStatus(id, data)
	default:
		return nil, unimplementedPacketErr(typ)
	}
}

// readAt reads len(buf) bytes from the remote file indicated by handle starting
// from offset.
func (c *Client) readAt(handle string, offset uint64, buf []byte) (uint32, error) {
	type packet struct {
		Type   byte
		Id     uint32
		Handle string
		Offset uint64
		Len    uint32
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type:   ssh_FXP_READ,
		Id:     id,
		Handle: handle,
		Offset: offset,
		Len:    uint32(len(buf)),
	}); err != nil {
		return 0, err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return 0, err
	}
	switch typ {
	case ssh_FXP_DATA:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return 0, &unexpectedIdErr{id, sid}
		}
		l, data := unmarshalUint32(data)
		n := copy(buf, data[:l])
		return uint32(n), nil
	case ssh_FXP_STATUS:
		return 0, eofOrErr(unmarshalStatus(id, data))
	default:
		return 0, unimplementedPacketErr(typ)
	}
}

// close closes a handle handle previously returned in the response
// to SSH_FXP_OPEN or SSH_FXP_OPENDIR. The handle becomes invalid
// immediately after this request has been sent.
func (c *Client) close(handle string) error {
	type packet struct {
		Type   byte
		Id     uint32
		Handle string
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type:   ssh_FXP_CLOSE,
		Id:     id,
		Handle: handle,
	}); err != nil {
		return err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return err
	}
	switch typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, data))
	default:
		return unimplementedPacketErr(typ)
	}
}

func (c *Client) fstat(handle string) (*attr, error) {
	type packet struct {
		Type   byte
		Id     uint32
		Handle string
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type:   ssh_FXP_FSTAT,
		Id:     id,
		Handle: handle,
	}); err != nil {
		return nil, err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return nil, err
	}
	switch typ {
	case ssh_FXP_ATTRS:
		sid, data := unmarshalUint32(data)
		if sid != id {
			return nil, &unexpectedIdErr{id, sid}
		}
		attr, _ := unmarshalAttrs(data)
		return attr, nil
	case ssh_FXP_STATUS:
		return nil, unmarshalStatus(id, data)
	default:
		return nil, unimplementedPacketErr(typ)
	}
}

// Remove removes the named file or directory.
func (c *Client) Remove(path string) error {
	// TODO(dfc) can't handle directories, yet
	type packet struct {
		Type     byte
		Id       uint32
		Filename string
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type:     ssh_FXP_REMOVE,
		Id:       id,
		Filename: path,
	}); err != nil {
		return err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return err
	}
	switch typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, data))
	default:
		return unimplementedPacketErr(typ)
	}
}

// Rename renames a file.
func (c *Client) Rename(oldname, newname string) error {
	type packet struct {
		Type             byte
		Id               uint32
		Oldpath, Newpath string
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type:    ssh_FXP_RENAME,
		Id:      id,
		Oldpath: oldname,
		Newpath: newname,
	}); err != nil {
		return err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return err
	}
	switch typ {
	case ssh_FXP_STATUS:
		return okOrErr(unmarshalStatus(id, data))
	default:
		return unimplementedPacketErr(typ)
	}
}

// writeAt writes len(buf) bytes from the remote file indicated by handle starting
// from offset.
func (c *Client) writeAt(handle string, offset uint64, buf []byte) (uint32, error) {
	type packet struct {
		Type   byte
		Id     uint32
		Handle string
		Offset uint64
		Length uint32
		Data   []byte
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextId()
	if err := sendPacket(c.w, packet{
		Type:   ssh_FXP_WRITE,
		Id:     id,
		Handle: handle,
		Offset: offset,
		Length: uint32(len(buf)),
		Data:   buf,
	}); err != nil {
		return 0, err
	}
	typ, data, err := recvPacket(c.r)
	if err != nil {
		return 0, err
	}
	switch typ {
	case ssh_FXP_STATUS:
		if err := okOrErr(unmarshalStatus(id, data)); err != nil {
			return 0, nil
		}
		return uint32(len(buf)), nil
	default:
		return 0, unimplementedPacketErr(typ)
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
	n, err := f.c.readAt(f.handle, f.offset, b)
	f.offset += uint64(n)
	return int(n), err
}

// ReadAt reads len(b) bytes from the File starting at byte offset off. It
// returns the number of bytes read and the error, if any. ReadAt always
// returns a non-nil error when n < len(b). At end of file, that error is
// io.EOF.
func (f *File) ReadAt(b []byte, off int64) (int, error) {
	n, err := f.c.readAt(f.handle, uint64(off), b)
	return int(n), err
}

// Stat returns the FileInfo structure describing file. If there is an
// error.
func (f *File) Stat() (os.FileInfo, error) {
	fi, err := f.c.fstat(f.handle)
	if err == nil {
		fi.name = path.Base(f.path)
	}
	return fi, err
}

// Write writes len(b) bytes to the File. It returns the number of bytes
// written and an error, if any. Write returns a non-nil error when n !=
// len(b).
func (f *File) Write(b []byte) (int, error) {
	n, err := f.c.writeAt(f.handle, f.offset, b)
	f.offset += uint64(n)
	return int(n), err
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
