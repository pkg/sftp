package sftp

import (
	"encoding"
	"io"
	"io/ioutil"
	"sync"

	"github.com/pkg/errors"
)

// Server takes the dataHandler and openHandler as arguments
// starts up packet handlers
// packet handlers convert packets to datas
// call dataHandler with data
// is done with packet/data
//
// dataHandler should call Handler() on data to process data and
// reply to client
//
// tricky bit about reading/writing spinning up workers to handle all packets

// datas using Id for switch
// + only 1 type + const
// - duplicates sftp prot Id

// datas using data-type for switch
// + types as types
// + type.Handle could enforce type of arg
// - requires dummy interface only for typing

type Request struct {
	Method   string
	Filepath string
	Pflags   uint32
	Attrs    uint32
	Target   string // for renames and sym-links
	packet   interface{}
}

// callback method for handling requests
type requestHandler func(Request) (interface{}, error)

type handleHandler func(string) (string, error)

// Server that abstracts the sftp protocol for a http request-like protocol
type RequestServer struct {
	// new
	requestHandler requestHandler
	getHandle      handleHandler
	getPath        handleHandler
	// same
	serverConn
	debugStream io.Writer
	pktChan     chan rxPacket
	maxTxPacket uint32
	// handles are just file paths
	openHandles map[string]string
	handleCount int
}

var handles map[string]string

func getHandle(path string) (string, error) {
	handles[path] = path
	return path, nil
}

func getPath(handle string) (string, error) {
	if path, ok := handles[handle]; ok { return path, nil }
	return "", errors.New("Missing handle")
}

// simple factory function
// one server per user-session
func NewRequestServer(handler requestHandler,
	rwc io.ReadWriteCloser) (*RequestServer, error) {

	s := &RequestServer{
		requestHandler: handler,
		getHandle:      getHandle,
		getPath:        getPath,
		serverConn: serverConn{
			conn: conn{
				Reader:      rwc,
				WriteCloser: rwc,
			},
		},
		debugStream: ioutil.Discard,
		pktChan:     make(chan rxPacket, sftpServerWorkerCount),
		openHandles: make(map[string]string),
		maxTxPacket: 1 << 15,
	}

	return s, nil
}

// start serving requests from user session
func (svr *RequestServer) Serve() error {
	var wg sync.WaitGroup
	wg.Add(sftpServerWorkerCount)
	for i := 0; i < sftpServerWorkerCount; i++ {
		go func() {
			defer wg.Done()
			if err := requestWorker(svr); err != nil {
				svr.conn.Close() // shuts down recvPacket
			}
		}()
	}

	var err error
	var pktType uint8
	var pktBytes []byte
	for {
		pktType, pktBytes, err = svr.recvPacket()
		if err != nil { break }
		svr.pktChan <- rxPacket{fxp(pktType), pktBytes}
	}

	close(svr.pktChan) // shuts down sftpServerWorkers
	wg.Wait()          // wait for all workers to exit
	return err
}

// make packet
// handle special cases
// convert to request
// call RequestHandler
// send feedback
func requestWorker(svr *RequestServer) error {
	for p := range svr.pktChan {
		pkt, err := makePacket(p)
		if err != nil { return err }

		var path string
		switch pkt := pkt.(type) {
		case *sshFxInitPacket:
			return svr.sendPacket(sshFxVersionPacket{sftpProtocolVersion, nil})
		case hasPath:
			path = pkt.getPath()
			svr.getHandle(path)
		case hasHandle:
			svr.getPath(pkt.getHandle())
		}
		request := makeRequest(pkt, path)
		handler, err := svr.requestHandler(request)
		if err != nil { return err }

		runHandler(svr, p, handler)
	}
	return nil
}

func runHandler(svr *RequestServer, p, handler interface{}) error {
	switch handler := handler.(type) {
	case io.Reader:
		data := make([]byte, svr.maxTxPacket)
		n, err := handler.Read(data)
		if err != nil && (err != io.EOF || n == 0) {
			return svr.sendError(p.(id), err)
		}
	case io.Writer:
	case FileActor:
	case Renamer:
	case ReadDirer:
	}
	return svr.sendError(p.(id), errors.New("undefined"))
}

func makeRequest(p interface{}, path string) Request {
	request := Request{Filepath: path}
	switch p.(type) {
	case *sshFxpReadPacket:
		request.Method = "Get"
	case *sshFxpWritePacket:
		request.Method = "Put"
	case *sshFxpReaddirPacket:
		request.Method = "List"
	case *sshFxpStatPacket, *sshFxpLstatPacket, *sshFxpFstatPacket,
		*sshFxpRealpathPacket, *sshFxpRemovePacket:
		request.Method = "Stat"
	case *sshFxpSetstatPacket, *sshFxpFsetstatPacket:
		request.Method = "Setstat"
	case *sshFxpMkdirPacket:
		request.Method = "Mkdir"
	case *sshFxpRmdirPacket:
		request.Method = "Rmdir"
	case *sshFxpRenamePacket:
		request.Method = "Rename"
	case *sshFxpSymlinkPacket:
		request.Method = "Symlink"
	case *sshFxpReadlinkPacket:
		request.Method = "Readlink"
	}
	return request
}

func deadcode(p interface{}) error {
	switch p := p.(type) {
	// these have no point of being here
	// but they need responding to
	case *sshFxpClosePacket:
	case *sshFxInitPacket:
	case *sshFxpExtendedPacket:
	// these need request
	case *sshFxpStatPacket, *sshFxpLstatPacket, *sshFxpFstatPacket,
		*sshFxpRealpathPacket, *sshFxpRemovePacket:
	case *sshFxpMkdirPacket:
	case *sshFxpRmdirPacket:
	case *sshFxpRenamePacket:
	case *sshFxpSymlinkPacket:
	case *sshFxpReadlinkPacket:
	case *sshFxpOpendirPacket:
	case *sshFxpReadPacket:
	case *sshFxpWritePacket:
	// this is a catch all interface
	case serverRespondablePacket:
	default:
		return errors.Errorf("unexpected packet type %T", p)
	}
	return nil
}

// all incoming packets
type packet interface {
	encoding.BinaryUnmarshaler
	id() uint32
}

// take raw incoming packet data and build packet objects
func makePacket(p rxPacket) (packet, error) {
	var pkt packet
	switch p.pktType {
	case ssh_FXP_INIT:
		pkt = &sshFxInitPacket{}
	case ssh_FXP_LSTAT:
		pkt = &sshFxpLstatPacket{}
	case ssh_FXP_OPEN:
		pkt = &sshFxpOpenPacket{}
	case ssh_FXP_CLOSE:
		pkt = &sshFxpClosePacket{}
	case ssh_FXP_READ:
		pkt = &sshFxpReadPacket{}
	case ssh_FXP_WRITE:
		pkt = &sshFxpWritePacket{}
	case ssh_FXP_FSTAT:
		pkt = &sshFxpFstatPacket{}
	case ssh_FXP_SETSTAT:
		pkt = &sshFxpSetstatPacket{}
	case ssh_FXP_FSETSTAT:
		pkt = &sshFxpFsetstatPacket{}
	case ssh_FXP_OPENDIR:
		pkt = &sshFxpOpendirPacket{}
	case ssh_FXP_READDIR:
		pkt = &sshFxpReaddirPacket{}
	case ssh_FXP_REMOVE:
		pkt = &sshFxpRemovePacket{}
	case ssh_FXP_MKDIR:
		pkt = &sshFxpMkdirPacket{}
	case ssh_FXP_RMDIR:
		pkt = &sshFxpRmdirPacket{}
	case ssh_FXP_REALPATH:
		pkt = &sshFxpRealpathPacket{}
	case ssh_FXP_STAT:
		pkt = &sshFxpStatPacket{}
	case ssh_FXP_RENAME:
		pkt = &sshFxpRenamePacket{}
	case ssh_FXP_READLINK:
		pkt = &sshFxpReadlinkPacket{}
	case ssh_FXP_SYMLINK:
		pkt = &sshFxpSymlinkPacket{}
	case ssh_FXP_EXTENDED:
		pkt = &sshFxpExtendedPacket{}
	default:
		return nil, errors.Errorf("unhandled packet type: %s", p.pktType)
	}
	if err := pkt.UnmarshalBinary(p.pktBytes); err != nil {
		return nil, err
	}
	return pkt, nil
}
