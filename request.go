package sftp

import (
	"io"
	"os"
	"path"
	"syscall"
)

// response passed back to packet handling code
type response struct {
	pkt resp_packet
	err error
}

type Request struct {
	// Get, Put, SetStat, Rename, Rmdir, Mkdir, Symlink, List, Stat, Readlink
	Method   string
	Filepath string
	Pflags   uint32
	Attrs    []byte // convert to sub-struct
	Target   string // for renames and sym-links
	// packet data
	pkt_id uint32
	data   []byte
	length uint32
	// reader/writer from handlers
	put_writer io.Writer
	get_reader io.Reader
}

// Here mainly to specify that Filepath is required
func newRequest(path string) *Request {
	request := &Request{Filepath: path}
	return request
}

// called from worker to handle packet/request
func (r *Request) handleRequest(handlers Handlers) response {
	var err error
	var rpkt resp_packet
	switch r.Method {
	case "Get":
		rpkt, err = fileget(handlers.FileGet, r)
	case "Put":
		rpkt, err = fileput(handlers.FilePut, r)
	case "SetStat", "Rename", "Rmdir", "Mkdir", "Symlink":
		rpkt, err = filecmd(handlers.FileCmd, r)
	case "List", "Stat", "Readlink":
		rpkt, err = fileinfo(handlers.FileInfo, r)
	}
	if err != nil { return response{nil, err} }
	return response{rpkt, nil}
}

// wrap FileReader handler
func fileget(h FileReader, r *Request) (resp_packet, error) {
	if r.get_reader == nil {
		reader, err := h.Fileread(r)
		if err != nil { return nil, syscall.EBADF }
		r.get_reader = reader
	}
	reader := r.get_reader
	data := make([]byte, clamp(r.length, maxTxPacket))
	n, err := reader.Read(data)
	if err != nil && (err != io.EOF || n == 0) { return nil, err }
	return &sshFxpDataPacket{
		ID:     r.pkt_id,
		Length: uint32(n),
		Data:   r.data[:n],
	}, nil
}

// wrap FileWriter handler
func fileput(h FileWriter, r *Request) (resp_packet, error) {
	if r.put_writer == nil {
		writer, err := h.Filewrite(r)
		if err != nil { return nil, syscall.EBADF }
		r.put_writer = writer
	}
	writer := r.put_writer

	_, err := writer.Write(r.data)
	if err != nil { return nil, err }
	return &sshFxpStatusPacket{
		ID: r.pkt_id,
		StatusError: StatusError{
			Code: ssh_FX_OK,
		}}, nil
}

// wrap FileCmder handler
func filecmd(h FileCmder, r *Request) (resp_packet, error) {
	err := h.Filecmd(r)
	if err != nil { return nil, err }
	return sshFxpStatusPacket{
		ID: r.pkt_id,
		StatusError: StatusError{
			Code: ssh_FX_OK,
		}}, nil
}

// wrap FileInfoer handler
func fileinfo(h FileInfoer, r *Request) (resp_packet, error) {
	finfo, err := h.Fileinfo(r)
	if err != nil { return nil, err }

	switch r.Method {
	case "List":
		dirname := path.Base(r.Filepath)
		ret := sshFxpNamePacket{ID: r.pkt_id}
		for _, fi := range finfo {
			ret.NameAttrs = append(ret.NameAttrs, sshFxpNameAttr{
				Name:     fi.Name(),
				LongName: runLs(dirname, fi),
				Attrs:    []interface{}{fi},
			})
		}
	case "Stat":
		if len(finfo) == 0 {
			err = &os.PathError{"stat", r.Filepath, syscall.ENOENT}
			return nil, err
		}
		return &sshFxpStatResponse{
			ID:   r.pkt_id,
			info: finfo[0],
		}, nil
	case "Readlink":
		if len(finfo) == 0 {
			err = &os.PathError{"readlink", r.Filepath, syscall.ENOENT}
			return nil, err
		}
		return sshFxpNamePacket{
			ID: r.pkt_id,
			NameAttrs: []sshFxpNameAttr{{
				Name:     finfo[0].Name(),
				LongName: finfo[0].Name(),
				Attrs:    emptyFileStat,
			}},
		}, nil
	}
	return nil, err
}

// populate attributes of request object from packet data
func (r *Request) populate(p interface{}) {
	// r.Filepath should be set in newRequest()
	switch p := p.(type) {
	case *sshFxpSetstatPacket:
		r.Method = "Setstat"
		r.Pflags = p.Flags
		r.Attrs = p.Attrs.([]byte)
		r.pkt_id = p.id()
	case *sshFxpFsetstatPacket:
		r.Method = "Setstat"
		r.Pflags = p.Flags
		r.Attrs = p.Attrs.([]byte)
		r.pkt_id = p.id()
	case *sshFxpRenamePacket:
		r.Method = "Rename"
		r.Target = p.Newpath
		r.pkt_id = p.id()
	case *sshFxpSymlinkPacket:
		r.Method = "Symlink"
		r.Target = p.Linkpath
		r.pkt_id = p.id()
	case *sshFxpReadPacket:
		r.Method = "Get"
		r.length = p.Len
		r.pkt_id = p.id()
	case *sshFxpWritePacket:
		r.Method = "Put"
		r.data = p.Data
		r.length = p.Length
		r.pkt_id = p.id()
	// below here method and path are all the data
	case *sshFxpReaddirPacket:
		r.Method = "List"
		r.pkt_id = p.id()
	case *sshFxpStatPacket, *sshFxpLstatPacket, *sshFxpFstatPacket,
		*sshFxpRealpathPacket, *sshFxpRemovePacket:
		r.Method = "Stat"
		r.pkt_id = p.(packet).id()
	case *sshFxpRmdirPacket:
		r.Method = "Rmdir"
		r.pkt_id = p.id()
	case *sshFxpReadlinkPacket:
		r.Method = "Readlink"
		r.pkt_id = p.id()
	// special cases
	case *sshFxpMkdirPacket:
		r.Method = "Mkdir"
		r.pkt_id = p.id()
		//r.Attrs are ignored in ./packet.go
	}
}
