package sftp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"sync"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
	"github.com/pkg/sftp/v2/encoding/ssh/filexfer/openssh"
	"github.com/pkg/sftp/v2/internal/pool"
)

var errInvalidHandle = errors.New("invalid handle")

type ServerHandler interface {
	Mkdir(ctx context.Context, req *sshfx.MkdirPacket) error
	Remove(ctx context.Context, req *sshfx.RemovePacket) error
	Rename(ctx context.Context, req *sshfx.RenamePacket) error
	Rmdir(ctx context.Context, req *sshfx.RmdirPacket) error
	SetStat(ctx context.Context, req *sshfx.SetStatPacket) error
	Symlink(ctx context.Context, req *sshfx.SymlinkPacket) error

	LStat(ctx context.Context, req *sshfx.LStatPacket) (*sshfx.Attributes, error)
	Stat(ctx context.Context, req *sshfx.StatPacket) (*sshfx.Attributes, error)

	ReadLink(ctx context.Context, req *sshfx.ReadLinkPacket) (string, error)
	RealPath(ctx context.Context, req *sshfx.RealPathPacket) (string, error)

	Open(ctx context.Context, req *sshfx.OpenPacket) (FileHandler, error)
	OpenDir(ctx context.Context, req *sshfx.OpenDirPacket) (DirHandler, error)
}

type HardlinkServerHandler interface {
	ServerHandler
	Hardlink(ctx context.Context, req *openssh.HardlinkExtendedPacket) error
}

type POSIXRenameServerHandler interface {
	ServerHandler
	POSIXRename(ctx context.Context, req *openssh.POSIXRenameExtendedPacket) error
}

type StatVFSServerHandler interface {
	ServerHandler
	StatVFS(ctx context.Context, req *openssh.StatVFSExtendedPacket) (*openssh.StatVFSExtendedReplyPacket, error)
}

type FileHandler interface {
	io.Closer
	io.ReaderAt
	io.WriterAt

	Name() string
	Handle() string
	Stat() (*sshfx.Attributes, error)
	SetStat(attrs *sshfx.Attributes) error
	Sync() error
}

type StatVFSFileHandler interface {
	FileHandler

	StatVFS() (*openssh.StatVFSExtendedReplyPacket, error)
}

type DirHandler interface {
	io.Closer

	Name() string
	Handle() string
	ReadDir(maxDataLen uint32) ([]*sshfx.NameEntry, error)
}

type wrapHandler func(ctx context.Context, req sshfx.Packet) (sshfx.Packet, error)

type Server struct {
	Handler ServerHandler

	MaxPacketLength int
	MaxDataLength   int
	Extensions      map[string]string

	Debug io.Writer

	wg      sync.WaitGroup
	handles sync.Map
	hijacks map[sshfx.PacketType]wrapHandler

	dataPktPool *pool.Pool[sshfx.DataPacket]

	mu       sync.Mutex
	shutdown chan struct{}
	err      error
}

func (srv *Server) GracefulStop() error {
	srv.mu.Lock()
	select {
	case <-srv.shutdown:
		srv.mu.Unlock()
		return fmt.Errorf("sftp: already shutting down")
	default:
		close(srv.shutdown)
	}
	srv.mu.Unlock()

	srv.wg.Wait()

	srv.handles.Range(func(k, v any) bool {
		handle, _ := k.(string)
		f, _ := v.(interface{ Name() string })

		fmt.Fprintf(srv.Debug, "sftp server file with handle %q left open: %T", handle, f.Name())

		return true
	})

	return srv.err
}

func (srv *Server) handshake(conn io.ReadWriter, maxPktLen uint32) error {
	b := make([]byte, maxPktLen)

	var initPkt sshfx.InitPacket
	if err := initPkt.ReadFrom(conn, b, maxPktLen); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}

	verPkt := sshfx.VersionPacket{
		Version: sftpProtocolVersion,
	}

	sshfx.RegisterExtendedPacketType[openssh.FSyncExtendedPacket]()
	verPkt.Extensions = append(verPkt.Extensions, openssh.ExtensionFSync())

	if _, ok := srv.Handler.(HardlinkServerHandler); ok {
		sshfx.RegisterExtendedPacketType[openssh.HardlinkExtendedPacket]()
		verPkt.Extensions = append(verPkt.Extensions, openssh.ExtensionHardlink())
	}

	if _, ok := srv.Handler.(POSIXRenameServerHandler); ok {
		sshfx.RegisterExtendedPacketType[openssh.POSIXRenameExtendedPacket]()
		verPkt.Extensions = append(verPkt.Extensions, openssh.ExtensionPOSIXRename())
	}

	if _, ok := srv.Handler.(StatVFSServerHandler); ok {
		sshfx.RegisterExtendedPacketType[openssh.StatVFSExtendedPacket]()
		verPkt.Extensions = append(verPkt.Extensions, openssh.ExtensionStatVFS())

		sshfx.RegisterExtendedPacketType[openssh.FStatVFSExtendedPacket]()
		verPkt.Extensions = append(verPkt.Extensions, openssh.ExtensionFStatVFS())
	}

	for name, data := range srv.Extensions {
		verPkt.Extensions = append(verPkt.Extensions, &sshfx.ExtensionPair{
			Name: name,
			Data: data,
		})
	}

	data, err := verPkt.MarshalBinary()
	if err != nil {
		return fmt.Errorf("marshal version: %w", err)
	}

	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("write version: %w", err)
	}

	return nil
}

func (srv *Server) FileFromHandle(handle string) (FileHandler, error) {
	f, _ := srv.handles.Load(handle)
	file, _ := f.(FileHandler)
	if file == nil {
		return nil, errInvalidHandle
	}
	return file, nil
}

func (srv *Server) DirFromHandle(handle string) (DirHandler, error) {
	f, _ := srv.handles.Load(handle)
	file, _ := f.(DirHandler)
	if file == nil {
		return nil, errInvalidHandle
	}
	return file, nil
}

/* func WithResponseExtension[REQ interface{ sshfx.Packet; sshfx.ExtendedData }, RESP sshfx.Packet](srv *Server, ext sshfx.ExtensionPair, fn func(context.Context, REQ) error) error {
	wrap := wrapHandler(func(ctx context.Context, req sshfx.Packet) (sshfx.Packet, error) {

	})

	return nil
} */

func Hijack[REQ sshfx.Packet](srv *Server, fn func(context.Context, REQ) error) error {
	wrap := wrapHandler(func(ctx context.Context, req sshfx.Packet) (sshfx.Packet, error) {
		return nil, fn(ctx, req.(REQ))
	})

	var pkt REQ

	return srv.register(pkt.Type(), wrap)
}

func HijackWithResponse[REQ, RESP sshfx.Packet](srv *Server, fn func(context.Context, REQ) (RESP, error)) error {
	wrap := wrapHandler(func(ctx context.Context, req sshfx.Packet) (sshfx.Packet, error) {
		return fn(ctx, req.(REQ))
	})

	var pkt REQ

	return srv.register(pkt.Type(), wrap)
}

func (srv *Server) register(typ sshfx.PacketType, wrap wrapHandler) error {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	if srv.shutdown != nil {
		return fmt.Errorf("sftp server: cannot register after starting server")
	}

	if _, exists := srv.hijacks[typ]; exists {
		return fmt.Errorf("sftp server: handler already registered for type: %s", typ)
	}

	srv.hijacks[typ] = wrap
	return nil
}

func (srv *Server) Serve(conn io.ReadWriteCloser) error {
	srv.mu.Lock()
	if srv.shutdown != nil {
		return fmt.Errorf("sftp: already serving")
	}
	srv.shutdown = make(chan struct{})
	srv.mu.Unlock()

	if srv.MaxDataLength < 0 {
		return fmt.Errorf("sftp: max data length cannot be negative: %d", srv.MaxDataLength)
	}

	if uint(srv.MaxDataLength) > math.MaxUint32 {
		return fmt.Errorf("sftp: max data length too large: %d", srv.MaxDataLength)
	}

	maxDataLen := uint32(srv.MaxDataLength)
	if maxDataLen == 0 {
		maxDataLen = sshfx.DefaultMaxDataLength
	}

	if srv.MaxPacketLength < 0 {
		return fmt.Errorf("sftp: max packet length cannot be negative: %d", srv.MaxDataLength)
	}

	if uint(srv.MaxPacketLength) > math.MaxUint32 {
		return fmt.Errorf("sftp: max packet length too large: %d", srv.MaxPacketLength)
	}

	maxPktLen := uint32(srv.MaxPacketLength)
	if maxPktLen == 0 {
		maxPktLen = maxDataLen + sshfx.MaxPacketLengthOverhead
	}

	if maxPktLen < maxDataLen {
		return fmt.Errorf("sftp: max packet length shorter than max data length: %d < %d", maxPktLen, maxDataLen)
	}

	if err := srv.handshake(conn, maxPktLen); err != nil {
		return fmt.Errorf("sftp: handshake failure: %w", err)
	}

	srv.wg.Add(1)
	defer func() {
		conn.Close()
		srv.wg.Done()
	}()

	var reqPkt sshfx.RequestPacket
	scratch := make([]byte, maxPktLen)
	dataHint := make([]byte, maxDataLen)
	outHint := make([]byte, maxPktLen)

	srv.dataPktPool = pool.NewPool[sshfx.DataPacket](64)

	for {
		select {
		case <-srv.shutdown:
			return nil
		default:
		}

		reqPkt.Reset()

		var err error
		var resp sshfx.Packet

		if err = reqPkt.ReadFrom(conn, scratch, maxPktLen); err != nil {
			select {
			case <-srv.shutdown:
				return nil
			default:
			}

			switch {
			case errors.Is(err, sshfx.StatusBadMessage):
				// Respond back with the StatusBadMessage, and continue on.

			case errors.Is(err, io.EOF):
				return nil

			default:
				return fmt.Errorf("sftp: read request packet: %w", err)
			}

		} else {
			resp, err = srv.handle(reqPkt.Request, dataHint, maxDataLen)
		}

		if resp == nil {
			resp = errorToStatus(err)
		}

		hdr, payload, err := resp.MarshalPacket(reqPkt.RequestID, outHint)
		if err != nil {
			return fmt.Errorf("sftp: marshal response packet: %w", err)
		}

		if _, err := conn.Write(hdr); err != nil {
			return fmt.Errorf("sftp: write packet: %w", err)
		}

		if len(payload) > 0 {
			if _, err := conn.Write(payload); err != nil {
				return fmt.Errorf("sftp: write packet payload: %w", err)
			}
		}

		switch resp := resp.(type) {
		case *sshfx.DataPacket:
			srv.dataPktPool.Put(resp)
		}
	}
}

func do[REQ any](srv *Server, req REQ, fn func(context.Context, REQ) error) (sshfx.Packet, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.wg.Add(1)
	defer srv.wg.Done()

	return nil, fn(ctx, req)
}

func get[REQ, RES any](srv *Server, req REQ, fn func(context.Context, REQ) (RES, error)) (RES, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.wg.Add(1)
	defer srv.wg.Done()

	return fn(ctx, req)
}

func (srv *Server) handle(req sshfx.Packet, hint []byte, maxDataLen uint32) (sshfx.Packet, error) {
	defer sshfx.PoolReturn(req)

	if len(srv.hijacks) > 0 {
		if fn := srv.hijacks[req.Type()]; fn != nil {
			return get(srv, req, fn)
		}
	}

	// Falling out of this switch returns err as an sshfx.StatusPacket.
	// Any other response packet type should return that packet directly, and not fall out of this switch.
	switch req := req.(type) {
	case *sshfx.MkdirPacket:
		return do(srv, req, srv.Handler.Mkdir)

	case *sshfx.RemovePacket:
		return do(srv, req, srv.Handler.Remove)

	case *sshfx.RenamePacket:
		return do(srv, req, srv.Handler.Rename)

	case *sshfx.RmdirPacket:
		return do(srv, req, srv.Handler.Rmdir)

	case *sshfx.SetStatPacket:
		return do(srv, req, srv.Handler.SetStat)

	case *sshfx.SymlinkPacket:
		return do(srv, req, srv.Handler.Symlink)

	case *sshfx.LStatPacket:
		attrs, err := get(srv, req, srv.Handler.LStat)
		if err != nil {
			return nil, err
		}

		return &sshfx.AttrsPacket{
			Attrs: *attrs,
		}, nil

	case *sshfx.StatPacket:
		attrs, err := get(srv, req, srv.Handler.Stat)
		if err != nil {
			return nil, err
		}

		return &sshfx.AttrsPacket{
			Attrs: *attrs,
		}, nil

	case *sshfx.ReadLinkPacket:
		name, err := get(srv, req, srv.Handler.ReadLink)
		if err != nil {
			return nil, err
		}

		return &sshfx.PathPseudoPacket{
			Path: name,
		}, nil

	case *sshfx.RealPathPacket:
		name, err := get(srv, req, srv.Handler.RealPath)
		if err != nil {
			return nil, err
		}

		return &sshfx.PathPseudoPacket{
			Path: name,
		}, nil

	case *sshfx.ExtendedPacket:
		// specially: falling out of this switch statement returns an SSH_FX_OP_UNSUPPORTED response.
		switch req := req.Data.(type) {
		case *openssh.POSIXRenameExtendedPacket:
			if renamer, ok := srv.Handler.(POSIXRenameServerHandler); ok {
				return do(srv, req, renamer.POSIXRename)
			}

		case *openssh.HardlinkExtendedPacket:
			if hardlinker, ok := srv.Handler.(HardlinkServerHandler); ok {
				return do(srv, req, hardlinker.Hardlink)
			}

		case *openssh.StatVFSExtendedPacket:
			if statvfser, ok := srv.Handler.(StatVFSServerHandler); ok {
				return get(srv, req, statvfser.StatVFS)
			}

		case interface{ GetHandle() string }:
			f, _ := srv.handles.Load(req.GetHandle())
			file, _ := f.(FileHandler)
			if file == nil {
				return nil, errInvalidHandle
			}

			switch req.(type) {
			case *openssh.FSyncExtendedPacket:
				return nil, file.Sync()

			case *openssh.FStatVFSExtendedPacket:
				if statvfser, ok := file.(StatVFSFileHandler); ok {
					return statvfser.StatVFS()
				}

				if statvfser, ok := srv.Handler.(StatVFSServerHandler); ok {
					req := &openssh.StatVFSExtendedPacket{
						Path: file.Name(),
					}

					return get(srv, req, statvfser.StatVFS)
				}
			}
		}

		if _, ok := req.Data.(*sshfx.Buffer); ok {
			// Return a different message when it is entirely unregisted into the system.
			// This allows one to more easily identify the sitaution.
			return nil, &sshfx.StatusPacket{
				StatusCode:   sshfx.StatusOpUnsupported,
				ErrorMessage: fmt.Sprintf("unregistered extended packet: %s", req.ExtendedRequest),
			}
		}

		return nil, &sshfx.StatusPacket{
			StatusCode:   sshfx.StatusOpUnsupported,
			ErrorMessage: fmt.Sprintf("unsupported extended packet: %s", req.ExtendedRequest),
		}

	case *sshfx.OpenPacket:
		file, err := get(srv, req, srv.Handler.Open)
		if err != nil {
			return nil, err
		}

		handle := file.Handle()

		srv.handles.Store(handle, file)

		return &sshfx.HandlePacket{
			Handle: handle,
		}, nil

	case *sshfx.OpenDirPacket:
		file, err := get(srv, req, srv.Handler.OpenDir)
		if err != nil {
			return nil, err
		}

		handle := file.Handle()

		srv.handles.Store(handle, file)

		return &sshfx.HandlePacket{
			Handle: handle,
		}, nil

	case *sshfx.ClosePacket:
		f, _ := srv.handles.LoadAndDelete(req.Handle)
		file, _ := f.(io.Closer)
		if file == nil {
			return nil, errInvalidHandle
		}

		return nil, file.Close()

	case *sshfx.ReadDirPacket:
		dir, err := srv.DirFromHandle(req.Handle)
		if err != nil {
			return nil, err
		}

		entries, err := dir.ReadDir(maxDataLen)
		if err != nil {
			if !errors.Is(err, io.EOF) || len(entries) == 0 {
				return nil, err
			}
		}

		return &sshfx.NamePacket{Entries: entries}, nil

	case interface{ GetHandle() string }:
		file, err := srv.FileFromHandle(req.GetHandle())
		if err != nil {
			return nil, err
		}

		switch req := req.(type) {
		case *sshfx.ReadPacket:
			if req.Length > maxDataLen {
				return nil, fmt.Errorf("read length request too large: %d", req.Length)
			}

			n, err := file.ReadAt(hint, int64(req.Offset))
			if err != nil {
				if !errors.Is(err, io.EOF) || n == 0 {
					return nil, err
				}
			}

			pkt := srv.dataPktPool.Get()
			pkt.Data = hint[:n]

			return pkt, nil

		case *sshfx.WritePacket:
			n, err := file.WriteAt(req.Data, int64(req.Offset))
			if err != nil {
				return nil, err
			}

			if n != len(req.Data) {
				return nil, io.ErrShortWrite
			}

			return nil, nil

		case *sshfx.FStatPacket:
			attrs, err := file.Stat()
			if err != nil {
				return nil, err
			}

			return &sshfx.AttrsPacket{Attrs: *attrs}, nil

		case *sshfx.FSetStatPacket:
			return nil, file.SetStat(&req.Attrs)
		}
	}

	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprintf("unsupported packet: %s", req.Type()),
	}
}

// These are so common, and they're all the same, just use a global singleton.
var (
	statusOK  = &sshfx.StatusPacket{StatusCode: sshfx.StatusOK}
	statusEOF = &sshfx.StatusPacket{StatusCode: sshfx.StatusEOF, ErrorMessage: io.EOF.Error()}
)

func errorToStatus(err error) sshfx.Packet {
	if err == nil {
		return statusOK
	}

	if errors.Is(err, io.EOF) {
		return statusEOF
	}

	var status *sshfx.StatusPacket
	if errors.As(err, &status) {
		return status
	}

	status = &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusFailure,
		ErrorMessage: err.Error(),
	}

	switch {
	case errors.As(err, &status.StatusCode):
		// Nothing to do, errors.As already assigned the value to statusPacket.
	case errors.Is(err, fs.ErrNotExist):
		status.StatusCode = sshfx.StatusNoSuchFile
	case syscallErrorAsStatus(err, status):
	}

	return status
}
