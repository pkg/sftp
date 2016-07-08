package sftp

type Request struct {
	Method   string
	Filepath string
	Pflags   uint32
	Attrs    []byte // convert to sub-struct
	Target   string // for renames and sym-links
	pktChan  chan packet
	cur_pkt  packet
	svr      RequestServer
}

func NewRequest(path string) *Request {
	request := &Request{Filepath: path}
	go request.requestWorker()
	return request
}

func (r *Request) sendError(err error) error {
	return r.svr.sendError(r.cur_pkt, err)
}

func (r *Request) close() { close(r.pktChan) }

func (r *Request) requestWorker() error {
	for p := range r.pktChan {
		r.populate(p)
		r.cur_pkt = p
	}
	return nil
}

func (r *Request) populate(p interface{}) {
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
	// special cases
	case *sshFxpReadPacket:
		r.Method = "Get"
		// data processed elsewhere
	case *sshFxpWritePacket:
		r.Method = "Put"
		// data processed elsewhere
	case *sshFxpMkdirPacket:
		r.Method = "Mkdir"
		// attributes are ignored in packet
	}
}
