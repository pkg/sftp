package sftp

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"syscall"
)

// Request contains the data and state for the incoming service request.
type Request struct {
	// Get, Put, SetStat, Stat, Rename, Remove
	// Rmdir, Mkdir, List, Readlink, Symlink
	Method   string
	Filepath string
	Attrs    []byte // convert to sub-struct
	Target   string // for renames and sym-links
	// packet data
	packets     []packet_data
	packetsLock sync.RWMutex
	// reader/writer from handlers
	put_writer io.WriterAt
	get_reader io.ReaderAt
	eof        bool // hack for readdir to keep eof state
}

type packet_data struct {
	id     uint32
	data   []byte
	length uint32
	offset int64
}

// Here mainly to specify that Filepath is required
func newRequest(path string) *Request {
	request := &Request{Filepath: filepath.Clean(path)}
	return request
}

// push packet_data into fifo
func (r *Request) pushPacket(pd packet_data) {
	r.packetsLock.Lock()
	defer r.packetsLock.Unlock()
	r.packets = append(r.packets, pd)
}

// pop packet_data into fifo
func (r *Request) popPacket() packet_data {
	r.packetsLock.Lock()
	defer r.packetsLock.Unlock()
	var pd packet_data
	pd, r.packets = r.packets[0], r.packets[1:]
	return pd
}

func (r *Request) pkt_id() uint32 {
	return r.packets[0].id
}

// called from worker to handle packet/request
func (r *Request) handle(handlers Handlers) (responsePacket, error) {
	var err error
	var rpkt responsePacket
	switch r.Method {
	case "Get":
		rpkt, err = fileget(handlers.FileGet, r)
	case "Put": // add "Append" to this to handle append only file writes
		rpkt, err = fileput(handlers.FilePut, r)
	case "SetStat", "Rename", "Rmdir", "Mkdir", "Symlink", "Remove":
		rpkt, err = filecmd(handlers.FileCmd, r)
	case "List", "Stat", "Readlink":
		rpkt, err = fileinfo(handlers.FileInfo, r)
	}
	return rpkt, err
}

// wrap FileReader handler
func fileget(h FileReader, r *Request) (responsePacket, error) {
	if r.get_reader == nil {
		reader, err := h.Fileread(r)
		if err != nil {
			return nil, syscall.EBADF
		}
		r.get_reader = reader
	}
	reader := r.get_reader

	pd := r.popPacket()
	data := make([]byte, clamp(pd.length, maxTxPacket))
	n, err := reader.ReadAt(data, pd.offset)
	if err != nil && (err != io.EOF || n == 0) {
		return nil, err
	}
	return &sshFxpDataPacket{
		ID:     pd.id,
		Length: uint32(n),
		Data:   data[:n],
	}, nil
}

// wrap FileWriter handler
func fileput(h FileWriter, r *Request) (responsePacket, error) {
	if r.put_writer == nil {
		writer, err := h.Filewrite(r)
		if err != nil {
			return nil, syscall.EBADF
		}
		r.put_writer = writer
	}
	writer := r.put_writer

	pd := r.popPacket()
	_, err := writer.WriteAt(pd.data, pd.offset)
	if err != nil {
		return nil, err
	}
	return &sshFxpStatusPacket{
		ID: pd.id,
		StatusError: StatusError{
			Code: ssh_FX_OK,
		}}, nil
}

// wrap FileCmder handler
func filecmd(h FileCmder, r *Request) (responsePacket, error) {
	err := h.Filecmd(r)
	if err != nil {
		return nil, err
	}
	return &sshFxpStatusPacket{
		ID: r.pkt_id(),
		StatusError: StatusError{
			Code: ssh_FX_OK,
		}}, nil
}

// wrap FileInfoer handler
func fileinfo(h FileInfoer, r *Request) (responsePacket, error) {
	if r.eof {
		return nil, io.EOF
	}
	finfo, err := h.Fileinfo(r)
	if err != nil {
		return nil, err
	}

	switch r.Method {
	case "List":
		dirname := path.Base(r.Filepath)
		ret := &sshFxpNamePacket{ID: r.pkt_id()}
		for _, fi := range finfo {
			ret.NameAttrs = append(ret.NameAttrs, sshFxpNameAttr{
				Name:     fi.Name(),
				LongName: runLs(dirname, fi),
				Attrs:    []interface{}{fi},
			})
		}
		r.eof = true
		return ret, nil
	case "Stat":
		if len(finfo) == 0 {
			err = &os.PathError{Op: "stat", Path: r.Filepath,
				Err: syscall.ENOENT}
			return nil, err
		}
		return &sshFxpStatResponse{
			ID:   r.pkt_id(),
			info: finfo[0],
		}, nil
	case "Readlink":
		if len(finfo) == 0 {
			err = &os.PathError{Op: "readlink", Path: r.Filepath,
				Err: syscall.ENOENT}
			return nil, err
		}
		filename := finfo[0].Name()
		return &sshFxpNamePacket{
			ID: r.pkt_id(),
			NameAttrs: []sshFxpNameAttr{{
				Name:     filename,
				LongName: filename,
				Attrs:    emptyFileStat,
			}},
		}, nil
	}
	return nil, err
}

// populate attributes of request object from packet data
func (r *Request) populate(p interface{}) {
	// r.Filepath should already be set
	var pd packet_data
	switch p := p.(type) {
	case *sshFxpSetstatPacket:
		r.Method = "Setstat"
		r.Attrs = p.Attrs.([]byte)
		pd.id = p.id()
	case *sshFxpFsetstatPacket:
		r.Method = "Setstat"
		r.Attrs = p.Attrs.([]byte)
		pd.id = p.id()
	case *sshFxpRenamePacket:
		r.Method = "Rename"
		r.Target = filepath.Clean(p.Newpath)
		pd.id = p.id()
	case *sshFxpSymlinkPacket:
		r.Method = "Symlink"
		r.Target = filepath.Clean(p.Linkpath)
		pd.id = p.id()
	case *sshFxpReadPacket:
		r.Method = "Get"
		pd.length = p.Len
		pd.offset = int64(p.Offset)
		pd.id = p.id()
	case *sshFxpWritePacket:
		r.Method = "Put"
		pd.id = p.id()
		pd.data = p.Data
		pd.length = p.Length
		pd.offset = int64(p.Offset)
	case *sshFxpReaddirPacket:
		r.Method = "List"
		pd.id = p.id()
	case *sshFxpRemovePacket:
		r.Method = "Remove"
		pd.id = p.id()
	case *sshFxpStatPacket, *sshFxpLstatPacket, *sshFxpFstatPacket:
		r.Method = "Stat"
		pd.id = p.(packet).id()
	case *sshFxpRmdirPacket:
		r.Method = "Rmdir"
		pd.id = p.id()
	case *sshFxpReadlinkPacket:
		r.Method = "Readlink"
		pd.id = p.id()
	case *sshFxpMkdirPacket:
		r.Method = "Mkdir"
		pd.id = p.id()
		//r.Attrs are ignored in ./packet.go
	}
	r.pushPacket(pd)
}
