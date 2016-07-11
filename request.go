package sftp

import (
	"io"
	"os"
	"path"
	"syscall"
)

// response passed back to packet handling code
type response_packet struct {
	pkt packet
	err error
}

// Valid Method values:
// Get, Put, SetStat, Rename, Rmdir, Mkdir, Symlink, List, Stat, Readlink
type Request struct {
	Method   string
	Filepath string
	Pflags   uint32
	Attrs    []byte // convert to sub-struct
	Target   string // for renames and sym-links
	pktChan  chan packet
	rspChan  chan response_packet
	svr      *RequestServer
}

func newRequest(path string, svr *RequestServer) *Request {
	request := &Request{Filepath: path, svr: svr}
	go request.requestWorker()
	return request
}

func (r *Request) close() {
	close(r.pktChan)
	close(r.rspChan)
}

func (r *Request) requestWorker() {
	for p := range r.pktChan {
		r.populate(p)
		handlers := r.svr.Handlers
		var err error
		switch r.Method {
		case "Get":
			pkt := p.(*sshFxpReadPacket)
			err = fileget(handlers.FileGet, r, pkt)
		case "Put":
			pkt := p.(*sshFxpWritePacket)
			err = fileput(handlers.FilePut, r, pkt)
		case "SetStat", "Rename", "Rmdir", "Mkdir", "Symlink":
			err = filecmd(handlers.FileCmd, r)
		case "List", "Stat", "Readlink":
			pkt := p.(packet)
			err = fileinfo(handlers.FileInfo, r, pkt)
		case "Open": // no-op
		}
		if err != nil { r.rspChan <- response_packet{nil, err} }
	}
}

func fileget(h FileReader, r *Request, pkt *sshFxpReadPacket) error {
	reader, err := h.Fileread(r)
	if err != nil { return syscall.EBADF }
	data := make([]byte, clamp(pkt.Len, maxTxPacket))
	n, err := reader.Read(data)
	if err != nil && (err != io.EOF || n == 0) { return err }
	return r.svr.sendPacket(sshFxpDataPacket{
		ID:     pkt.ID,
		Length: uint32(n),
		Data:   data[:n],
	})
}
func fileput(h FileWriter, r *Request, pkt *sshFxpWritePacket) error {
	writer, err := h.Filewrite(r)
	if err != nil { return syscall.EBADF }
	_, err = writer.Write(pkt.Data)
	return err
}
func filecmd(h FileCmder, r *Request) error {
	err := h.Filecmd(r)
	return err
}
func fileinfo(h FileInfoer, r *Request, pkt packet) error {
	finfo, err := h.Fileinfo(r)
	if err != nil { return err }

	switch r.Method {
	case "List":
		dirname := path.Base(r.Filepath)
		ret := sshFxpNamePacket{ID: pkt.id()}
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
			return err
		}
		return r.svr.sendPacket(sshFxpStatResponse{
			ID:   pkt.id(),
			info: finfo[0],
		})
	case "Readlink":
		if len(finfo) == 0 {
			err = &os.PathError{"readlink", r.Filepath, syscall.ENOENT}
			return err
		}
		return r.svr.sendPacket(sshFxpNamePacket{
			ID: pkt.id(),
			NameAttrs: []sshFxpNameAttr{{
				Name:     finfo[0].Name(),
				LongName: finfo[0].Name(),
				Attrs:    emptyFileStat,
			}},
		})
	}
	return err
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
