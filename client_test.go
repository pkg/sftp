package sftp

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"sync/atomic"
	"testing"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
	"github.com/pkg/sftp/v2/internal/sync"
)

func TestClient(t *testing.T) {
	type allFile interface {
		fs.File

		// Is it impossible to implement this properly?
		// It is a protocol error to attempt to use an ordinary file handle returned by SSH_FXP_OPEN.
		// And, fs.FS only permits a simple `Open()`.
		// fs.ReadDirFile
	}

	var _ allFile = new(File)

	type allDir interface {
		io.Closer
		ReadDir(n int) ([]fs.DirEntry, error)
	}

	var _ allDir = new(Dir)

	type allFS interface {
		fs.FS
		// fs.GlobFS
		fs.ReadDirFS
		fs.ReadFileFS
		fs.StatFS
		fs.SubFS
	}

	var _ allFS = new(fsys)
}

type sink struct{}

func (sink) Close() error                { return nil }
func (sink) Write(p []byte) (int, error) { return len(p), nil }

func TestClientZeroLengthPacket(t *testing.T) {
	// Packet length zero (never valid). This used to crash the client.
	r := bytes.NewReader([]byte{0, 0, 0, 0})

	cl, err := NewClientPipe(t.Context(), r, sink{})
	if err == nil {
		t.Error("expected an error, got nil")
	}
	if cl != nil {
		cl.Close()
	}
}

func TestClientShortPacket(t *testing.T) {
	// init packet too short.
	r := bytes.NewReader([]byte{0, 0, 0, 1, 2})

	cl, err := NewClientPipe(t.Context(), r, sink{})
	if !errors.Is(err, sshfx.ErrShortPacket) {
		t.Fatalf("got error %#v, but expected sshfx.ErrShortPacket", err)
	}
	if cl != nil {
		cl.Close()
	}
}

type tickWriter struct {
	count atomic.Uint32
	steps atomic.Uint32

	mu     sync.Mutex
	once   sync.Once
	closed bool
	step   chan struct{}
}

func (t *tickWriter) wait() {
	_ = t.steps.Add(1)

	<-t.step
}

func (t *tickWriter) Write(b []byte) (written int, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return 0, io.ErrClosedPipe
	}

	_ = t.count.Add(1)

	t.step <- struct{}{}

	return len(b), nil
}

func (t *tickWriter) Close() error {
	t.once.Do(func() {
		t.mu.Lock()
		defer t.mu.Unlock()

		t.closed = true
		close(t.step)
	})
	return nil
}

func streamPackets(t testing.TB, verPkt *sshfx.VersionPacket, packets ...sshfx.PacketMarshaller) (io.ReadCloser, io.WriteCloser) {
	pr, pw := io.Pipe()

	ticker := &tickWriter{
		step: make(chan struct{}),
	}

	go func() {
		defer func() {
			for range ticker.step {
			}
		}()

		defer pw.Close()

		bindata, err := verPkt.MarshalBinary()
		if err != nil {
			t.Errorf("could not binary marshal %#v: %v", verPkt, err)
			return
		}

		ticker.wait()
		if _, err := pw.Write(bindata); err != nil {
			t.Error(err)
			return
		}

		var reqid uint32
		for _, packet := range packets {
			reqid++

			header, payload, err := packet.MarshalPacket(reqid, nil)
			if err != nil {
				t.Errorf("could not packet marshal %#v: %v", packet, err)
				return
			}

			ticker.wait()

			if _, err := pw.Write(header); err != nil {
				t.Error("could not write packet header:", err)
				return
			}

			if _, err := pw.Write(payload); err != nil {
				t.Error("could not write packet payload:", err)
				return
			}
		}
	}()

	return pr, ticker
}

type noSidBrokenPacket struct{}

func (noSidBrokenPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	return []byte{0, 0, 0, 10, 0, 0}, nil, nil
}

func (noSidBrokenPacket) MarshalSize() int {
	return 10
}

// Issue #418: panic in clientConn.recv when the sid is incomplete.
func TestClientNoSid(t *testing.T) {
	rd, wr := streamPackets(t,
		&sshfx.VersionPacket{
			Version: sftpProtocolVersion,
		},
		noSidBrokenPacket{},
	)

	cl, err := NewClientPipe(t.Context(), rd, wr)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	_, err = cl.Stat("anything")
	if !errors.Is(err, sshfx.StatusConnectionLost) {
		t.Errorf("cl.Stat = %q, expected sshfx.StatusConnectionLost", err)
	}
}

// sftp/issue/390 - server disconnect should not cause io.EOF or
// io.ErrUnexpectedEOF in sftp.File.Read, because those confuse io.ReadFull.
func TestClientRoughDisconnectEOF(t *testing.T) {
	rd, wr := streamPackets(t,
		&sshfx.VersionPacket{
			Version: sftpProtocolVersion,
		},
		&sshfx.HandlePacket{
			Handle: "foo",
		},
		&sshfx.DataPacket{
			Data: []byte("foo"),
		},
	)

	cl, err := NewClientPipe(t.Context(), rd, wr)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	f, err := cl.Open("anything")
	if err != nil {
		t.Fatal(err)
	}

	_, err = io.ReadFull(f, make([]byte, 10))
	if !errors.Is(err, sshfx.StatusConnectionLost) {
		t.Errorf("io.ReadFull error = %q, but wanted sshfx.StatusConnectionLost", err)
	}
}
