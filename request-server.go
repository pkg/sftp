package sftp

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
)

var maxTxPacket uint32 = 1 << 15

type handleHandler func(string) string

// Handlers contains the 4 SFTP server request handlers.
type Handlers struct {
	FileGet  FileReader
	FilePut  FileWriter
	FileCmd  FileCmder
	FileInfo FileInfoer
}

// RequestServer abstracts the sftp protocol with an http request-like protocol
type RequestServer struct {
	serverConn
	Handlers        Handlers
	pktChan         chan packet
	openRequests    map[string]*Request
	openRequestLock sync.RWMutex
	handleCount     int
}

// NewRequestServer creates/allocates/returns new RequestServer.
// Normally there there will be one server per user-session.
func NewRequestServer(rwc io.ReadWriteCloser, h Handlers) *RequestServer {
	return &RequestServer{
		serverConn: serverConn{
			conn: conn{
				Reader:      rwc,
				WriteCloser: rwc,
			},
		},
		Handlers:     h,
		pktChan:      make(chan packet, sftpServerWorkerCount),
		openRequests: make(map[string]*Request),
	}
}

func (rs *RequestServer) nextRequest(r *Request) string {
	rs.openRequestLock.Lock()
	defer rs.openRequestLock.Unlock()
	rs.handleCount++
	handle := strconv.Itoa(rs.handleCount)
	rs.openRequests[handle] = r
	return handle
}

func (rs *RequestServer) getRequest(handle string) (*Request, bool) {
	rs.openRequestLock.Lock()
	defer rs.openRequestLock.Unlock()
	r, ok := rs.openRequests[handle]
	return r, ok
}

func (rs *RequestServer) closeRequest(handle string) {
	rs.openRequestLock.Lock()
	defer rs.openRequestLock.Unlock()
	if _, ok := rs.openRequests[handle]; ok {
		delete(rs.openRequests, handle)
	}
}

// Close the read/write/closer to trigger exiting the main server loop
func (rs *RequestServer) Close() error { return rs.conn.Close() }

// Serve requests for user session
func (rs *RequestServer) Serve() error {
	var wg sync.WaitGroup
	wg.Add(sftpServerWorkerCount)
	for i := 0; i < sftpServerWorkerCount; i++ {
		go func() {
			defer wg.Done()
			if err := rs.packetWorker(); err != nil {
				rs.conn.Close() // shuts down recvPacket
			}
		}()
	}

	var err error
	var pktType uint8
	var pktBytes []byte
	for {
		pktType, pktBytes, err = rs.recvPacket()
		if err != nil {
			break
		}
		pkt, err := makePacket(rxPacket{fxp(pktType), pktBytes})
		if err != nil {
			break
		}
		rs.pktChan <- pkt
	}

	close(rs.pktChan) // shuts down sftpServerWorkers
	wg.Wait()         // wait for all workers to exit
	return err
}

func (rs *RequestServer) packetWorker() error {
	for pkt := range rs.pktChan {
		// fmt.Println("Incoming Packet: ", pkt, reflect.TypeOf(pkt))
		var handle string
		var rpkt responsePacket
		var err error
		switch pkt := pkt.(type) {
		case *sshFxInitPacket:
			rpkt = sshFxVersionPacket{sftpProtocolVersion, nil}
		case *sshFxpClosePacket:
			handle = pkt.getHandle()
			rs.closeRequest(handle)
			rpkt = statusFromError(pkt, nil)
		case *sshFxpRealpathPacket:
			rpkt = cleanPath(pkt)
		case isOpener:
			handle = rs.nextRequest(newRequest(pkt.getPath()))
			rpkt = sshFxpHandlePacket{pkt.id(), handle}
		case hasPath:
			handle = rs.nextRequest(newRequest(pkt.getPath()))
			rpkt = rs.request(handle, pkt)
		case hasHandle:
			handle = pkt.getHandle()
			rpkt = rs.request(handle, pkt)
		}

		// fmt.Println("Reply Packet: ", rpkt, reflect.TypeOf(rpkt))
		err = rs.sendPacket(rpkt)
		if err != nil {
			return err
		}
	}
	return nil
}

func cleanPath(pkt *sshFxpRealpathPacket) responsePacket {
	path := pkt.getPath()
	if !filepath.IsAbs(path) {
		path = "/" + path
	} // all paths are absolute

	cleaned_path := filepath.Clean(path)
	return &sshFxpNamePacket{
		ID: pkt.id(),
		NameAttrs: []sshFxpNameAttr{{
			Name:     cleaned_path,
			LongName: cleaned_path,
			Attrs:    emptyFileStat,
		}},
	}
}

func (rs *RequestServer) request(handle string, pkt packet) responsePacket {
	var rpkt responsePacket
	var err error
	if request, ok := rs.getRequest(handle); ok {
		// called here to keep packet handling out of request for testing
		request.populate(pkt)
		// fmt.Println("Request Method: ", request.Method)
		rpkt, err = request.handle(rs.Handlers)
		if err != nil {
			err = errorAdapter(err)
			rpkt = statusFromError(pkt, err)
		}
	} else {
		rpkt = statusFromError(pkt, syscall.EBADF)
	}
	return rpkt
}

// os.ErrNotExist should convert to ssh_FX_NO_SUCH_FILE, but is not recognized
// by statusFromError. So we convert to syscall.ENOENT which it does.
func errorAdapter(err error) error {
	if err == os.ErrNotExist {
		return syscall.ENOENT
	}
	return err
}
