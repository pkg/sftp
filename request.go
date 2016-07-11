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
	data     []byte
	length   uint32
	pktChan  chan packet
	rspChan  chan response
	handlers Handlers
}

func newRequest(path string, handlers Handlers) *Request {
	request := &Request{Filepath: path, handlers: handlers}
	go request.requestWorker()
	return request
}

func (r *Request) close() {
	close(r.pktChan)
	close(r.rspChan)
}

func (r *Request) requestWorker() {
	for pkt := range r.pktChan {
		r.populate(pkt)
		handlers := r.handlers
		var err error
		var rpkt resp_packet
		switch r.Method {
		case "Get":
			rpkt, err = fileget(handlers.FileGet, r, pkt.id())
		case "Put":
			rpkt, err = fileput(handlers.FilePut, r, pkt.id())
		case "SetStat", "Rename", "Rmdir", "Mkdir", "Symlink":
			rpkt, err = filecmd(handlers.FileCmd, r, pkt.id())
		case "List", "Stat", "Readlink":
			rpkt, err = fileinfo(handlers.FileInfo, r, pkt.id())
		case "Open": // no-op
		}
		if err != nil { r.rspChan <- response{nil, err} }
		r.rspChan <- response{rpkt, nil}
	}
}

func fileget(h FileReader, r *Request, pkt_id uint32) (resp_packet, error) {
	reader, err := h.Fileread(r)
	if err != nil { return nil, syscall.EBADF }
	data := make([]byte, clamp(r.length, maxTxPacket))
	n, err := reader.Read(data)
	if err != nil && (err != io.EOF || n == 0) { return nil, err }
	return &sshFxpDataPacket{
		ID:     pkt_id,
		Length: uint32(n),
		Data:   r.data[:n],
	}, nil
}
func fileput(h FileWriter, r *Request, pkt_id uint32) (resp_packet, error) {
	writer, err := h.Filewrite(r)
	if err != nil { return nil, syscall.EBADF }
	_, err = writer.Write(r.data)
	if err != nil { return nil, err }
	return &sshFxpStatusPacket{
		ID: pkt_id,
		StatusError: StatusError{
			Code: ssh_FX_OK,
		}}, nil
}
func filecmd(h FileCmder, r *Request, pkt_id uint32) (resp_packet, error) {
	err := h.Filecmd(r)
	if err != nil { return nil, err }
	return sshFxpStatusPacket{
		ID: pkt_id,
		StatusError: StatusError{
			Code: ssh_FX_OK,
		}}, nil
}
func fileinfo(h FileInfoer, r *Request, pkt_id uint32) (resp_packet, error) {
	finfo, err := h.Fileinfo(r)
	if err != nil { return nil, err }

	switch r.Method {
	case "List":
		dirname := path.Base(r.Filepath)
		ret := sshFxpNamePacket{ID: pkt_id}
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
			ID:   pkt_id,
			info: finfo[0],
		}, nil
	case "Readlink":
		if len(finfo) == 0 {
			err = &os.PathError{"readlink", r.Filepath, syscall.ENOENT}
			return nil, err
		}
		return sshFxpNamePacket{
			ID: pkt_id,
			NameAttrs: []sshFxpNameAttr{{
				Name:     finfo[0].Name(),
				LongName: finfo[0].Name(),
				Attrs:    emptyFileStat,
			}},
		}, nil
	}
	return nil, err
}

func (r *Request) populate(p interface{}) {
	// r.Filepath set in newRequest()
	switch p := p.(type) {
	case *sshFxpSetstatPacket:
		r.Method = "Setstat"
		r.Pflags = p.Flags
		r.Attrs = p.Attrs.([]byte)
	case *sshFxpFsetstatPacket:
		r.Method = "Setstat"
		r.Pflags = p.Flags
		r.Attrs = p.Attrs.([]byte)
	case *sshFxpRenamePacket:
		r.Method = "Rename"
		r.Target = p.Newpath
	case *sshFxpSymlinkPacket:
		r.Method = "Symlink"
		r.Target = p.Linkpath
	// below here method and path are all the data
	case *sshFxpReaddirPacket:
		r.Method = "List"
	case *sshFxpStatPacket, *sshFxpLstatPacket, *sshFxpFstatPacket,
		*sshFxpRealpathPacket, *sshFxpRemovePacket:
		r.Method = "Stat"
	case *sshFxpRmdirPacket:
		r.Method = "Rmdir"
	case *sshFxpReadlinkPacket:
		r.Method = "Readlink"
	case *sshFxpOpenPacket:
		r.Method = "Open"
	// special cases
	case *sshFxpReadPacket:
		r.Method = "Get"
		// data processed elsewhere
	case *sshFxpWritePacket:
		r.Method = "Put"
		// data processed elsewhere
	case *sshFxpMkdirPacket:
		r.Method = "Mkdir"
		//r.Attrs are ignored in ./packet.go
	}
}
