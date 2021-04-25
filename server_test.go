package sftp

import (
	"bytes"
	"io"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

const (
	typeDirectory = "d"
	typeFile      = "[^d]"
)

func TestRunLsWithExamplesDirectory(t *testing.T) {
	path := "examples"
	item, _ := os.Stat(path)
	result := runLs(path, item)
	runLsTestHelper(t, result, typeDirectory, path)
}

func TestRunLsWithLicensesFile(t *testing.T) {
	path := "LICENSE"
	item, _ := os.Stat(path)
	result := runLs(path, item)
	runLsTestHelper(t, result, typeFile, path)
}

/*
   The format of the `longname' field is unspecified by this protocol.
   It MUST be suitable for use in the output of a directory listing
   command (in fact, the recommended operation for a directory listing
   command is to simply display this data).  However, clients SHOULD NOT
   attempt to parse the longname field for file attributes; they SHOULD
   use the attrs field instead.

    The recommended format for the longname field is as follows:

        -rwxr-xr-x   1 mjos     staff      348911 Mar 25 14:29 t-filexfer
        1234567890 123 12345678 12345678 12345678 123456789012

   Here, the first line is sample output, and the second field indicates
   widths of the various fields.  Fields are separated by spaces.  The
   first field lists file permissions for user, group, and others; the
   second field is link count; the third field is the name of the user
   who owns the file; the fourth field is the name of the group that
   owns the file; the fifth field is the size of the file in bytes; the
   sixth field (which actually may contain spaces, but is fixed to 12
   characters) is the file modification time, and the seventh field is
   the file name.  Each field is specified to be a minimum of certain
   number of character positions (indicated by the second line above),
   but may also be longer if the data does not fit in the specified
   length.

    The SSH_FXP_ATTRS response has the following format:

        uint32     id
        ATTRS      attrs

   where `id' is the request identifier, and `attrs' is the returned
   file attributes as described in Section ``File Attributes''.
*/
func runLsTestHelper(t *testing.T, result, expectedType, path string) {
	// using regular expressions to make tests work on all systems
	// a virtual file system (like afero) would be needed to mock valid filesystem checks
	// expected layout is:
	// drwxr-xr-x   8 501      20            272 Aug  9 19:46 examples

	t.Log(result)

	sparce := strings.Split(result, " ")

	var fields []string
	for _, field := range sparce {
		if field == "" {
			continue
		}

		fields = append(fields, field)
	}

	perms, linkCnt, user, group, size := fields[0], fields[1], fields[2], fields[3], fields[4]
	dateTime := strings.Join(fields[5:8], " ")
	filename := fields[8]

	// permissions (len 10, "drwxr-xr-x")
	const (
		rwxs = "[-r][-w][-xsS]"
		rwxt = "[-r][-w][-xtT]"
	)
	if ok, err := regexp.MatchString("^"+expectedType+rwxs+rwxs+rwxt+"$", perms); !ok {
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("runLs(%q): permission field mismatch, expected dir, got: %#v, err: %#v", path, perms, err)
	}

	// link count (len 3, number)
	const (
		number = "(?:[0-9]+)"
	)
	if ok, err := regexp.MatchString("^"+number+"$", linkCnt); !ok {
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("runLs(%q): link count field mismatch, got: %#v, err: %#v", path, linkCnt, err)
	}

	// username / uid (len 8, number or string)
	const (
		name = "(?:[a-z_][a-z0-9_]*)"
	)
	if ok, err := regexp.MatchString("^(?:"+number+"|"+name+")+$", user); !ok {
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("runLs(%q): username / uid mismatch, expected user, got: %#v, err: %#v", path, user, err)
	}

	// groupname / gid (len 8, number or string)
	if ok, err := regexp.MatchString("^(?:"+number+"|"+name+")+$", group); !ok {
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("runLs(%q): groupname / gid mismatch, expected group, got: %#v, err: %#v", path, group, err)
	}

	// filesize (len 8)
	if ok, err := regexp.MatchString("^"+number+"$", size); !ok {
		if err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Errorf("runLs(%q): filesize field mismatch, expected size in bytes, got: %#v, err: %#v", path, size, err)
	}

	// mod time (len 12, e.g. Aug  9 19:46)
	_, err := time.Parse("Jan 2 15:04", dateTime)
	if err != nil {
		_, err = time.Parse("Jan 2 2006", dateTime)
		if err != nil {
			t.Errorf("runLs.dateTime = %#v should match `Jan 2 15:04` or `Jan 2 2006`: %+v", dateTime, err)
		}
	}

	// filename
	if path != filename {
		t.Errorf("runLs.filename = %#v, expected: %#v", filename, path)
	}
}

func clientServerPair(t *testing.T) (*Client, *Server) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	var options []ServerOption
	if *testAllocator {
		options = append(options, WithAllocator())
	}
	server, err := NewServer(struct {
		io.Reader
		io.WriteCloser
	}{sr, sw}, options...)
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve()
	client, err := NewClientPipe(cr, cw)
	if err != nil {
		t.Fatalf("%+v\n", err)
	}
	return client, server
}

const extensionBad = "notexist@example.net"

type BadExtendedPacket struct {
	Payload string
}

func (ep *BadExtendedPacket) Type() sshfx.PacketType {
	return sshfx.PacketTypeExtended
}

func (ep *BadExtendedPacket) MarshalPacket(reqid uint32, b []byte) (header, payload []byte, err error) {
	p := &sshfx.ExtendedPacket{
		ExtendedRequest: extensionBad,

		Data: ep,
	}

	return p.MarshalPacket(reqid, b)
}

func (ep *BadExtendedPacket) MarshalInto(buf *sshfx.Buffer) {
	buf.AppendString(ep.Payload)
}

func (ep *BadExtendedPacket) MarshalBinary() ([]byte, error) {
	size := 4 + len(ep.Payload)

	buf := sshfx.NewBuffer(make([]byte, 0, size))
	ep.MarshalInto(buf)
	return buf.Bytes(), nil
}

func (ep *BadExtendedPacket) UnmarshalFrom(buf *sshfx.Buffer) (err error) {
	if ep.Payload, err = buf.ConsumeString(); err != nil {
		return err
	}

	return nil
}

func (ep *BadExtendedPacket) UnmarshalBinary(data []byte) (err error) {
	return ep.UnmarshalFrom(sshfx.NewBuffer(data))
}

func checkServerAllocator(t *testing.T, server *Server) {
	if server.pktMgr.alloc == nil {
		return
	}
	checkAllocatorBeforeServerClose(t, server.pktMgr.alloc)
	server.Close()
	checkAllocatorAfterServerClose(t, server.pktMgr.alloc)
}

// test that errors are sent back when we request an invalid extended packet operation
// this validates the following rfc draft is followed https://tools.ietf.org/html/draft-ietf-secsh-filexfer-extensions-00
func TestInvalidExtendedPacket(t *testing.T) {
	client, server := clientServerPair(t)
	defer client.Close()
	defer server.Close()

	badPacket := &BadExtendedPacket{
		Payload: "foobar",
	}
	err := client.sendPacket(badPacket, nil)
	if err == nil {
		t.Fatal("expected error from sendPacket, but got none")
	}

	statusErr, ok := err.(*StatusError)
	if !ok {
		t.Fatalf("failed to convert error from sendPacket to *StatusError: %#v", err)
	}

	if code := sshfx.Status(statusErr.Code); code != sshfx.StatusOPUnsupported {
		t.Errorf("statusErr.Code = %s, wanted %s", code, sshfx.StatusOPUnsupported)
	}

	checkServerAllocator(t, server)
}

// test that server handles concurrent requests correctly
func TestConcurrentRequests(t *testing.T) {
	skipIfWindows(t)
	filename := "/etc/passwd"
	if runtime.GOOS == "plan9" {
		filename = "/lib/ndb/local"
	}
	client, server := clientServerPair(t)
	defer client.Close()
	defer server.Close()

	concurrency := 2
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < 1024; j++ {
				f, err := client.Open(filename)
				if err != nil {
					t.Errorf("failed to open file: %v", err)
					continue
				}
				if err := f.Close(); err != nil {
					t.Errorf("failed t close file: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	checkServerAllocator(t, server)
}

// Test error conversion
func TestStatusFromError(t *testing.T) {
	type test struct {
		err error
		pkt *sshFxpStatusPacket
	}
	tpkt := func(id, code uint32) *sshFxpStatusPacket {
		return &sshFxpStatusPacket{
			ID:          id,
			StatusError: StatusError{Code: code},
		}
	}
	testCases := []test{
		{syscall.ENOENT, tpkt(1, sshFxNoSuchFile)},
		{&os.PathError{Err: syscall.ENOENT},
			tpkt(2, sshFxNoSuchFile)},
		{&os.PathError{Err: errors.New("foo")}, tpkt(3, sshFxFailure)},
		{ErrSSHFxEOF, tpkt(4, sshFxEOF)},
		{ErrSSHFxOpUnsupported, tpkt(5, sshFxOPUnsupported)},
		{io.EOF, tpkt(6, sshFxEOF)},
		{os.ErrNotExist, tpkt(7, sshFxNoSuchFile)},
	}
	for _, tc := range testCases {
		tc.pkt.StatusError.msg = tc.err.Error()
		assert.Equal(t, tc.pkt, statusFromError(tc.pkt.ID, tc.err))
	}
}

// This was written to test a race b/w open immediately followed by a stat.
// Previous to this the Open would trigger the use of a worker pool, then the
// stat packet would come in an hit the pool and return faster than the open
// (returning a file-not-found error).
// The below by itself wouldn't trigger the race however, I needed to add a
// small sleep in the openpacket code to trigger the issue. I wanted to add a
// way to inject that in the code but right now there is no good place for it.
// I'm thinking after I convert the server into a request-server backend I
// might be able to do something with the runWorker method passed into the
// packet manager. But with the 2 implementations fo the server it just doesn't
// fit well right now.
func TestOpenStatRace(t *testing.T) {
	client, server := clientServerPair(t)
	defer client.Close()
	defer server.Close()

	ch := make(chan result, 3)

	// openpacket finishes to fast to trigger race in tests
	// need to add a small sleep on server to openpackets somehow
	tmppath := path.Join(os.TempDir(), "stat_race")
	defer os.Remove(tmppath)

	id1 := client.nextID()
	client.dispatchPacket(ch, id1, &sshfx.OpenPacket{
		Filename: tmppath,
		PFlags:   flags(os.O_RDWR | os.O_CREATE | os.O_TRUNC),
	})

	id2 := client.nextID()
	client.dispatchPacket(ch, id2, &sshfx.LStatPacket{
		Path: tmppath,
	})

	r := <-ch
	if r.err != nil {
		t.Fatal("unexpected error:", r.err)
	}

	switch r.pkt.RequestID {
	case id1:
	case id2:
		t.Fatal("race condition detected: LStat response returned first")
	default:
		t.Fatal("unexpected id in response:", r.pkt.RequestID)
	}

	switch r.pkt.PacketType {
	case sshfx.PacketTypeHandle:
		// First, we should get the Open response: Handle
		var handle sshfx.HandlePacket

		if err := handle.UnmarshalPacketBody(&r.pkt.Data); err != nil {
			t.Fatal("unexpected error:", err)
		}

		defer client.close(handle.Handle)

	case sshfx.PacketTypeStatus:
		var status sshfx.StatusPacket

		if err := status.UnmarshalPacketBody(&r.pkt.Data); err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Fatal("unexpected status packet:", status)

	default:
		t.Fatal("unexpected type:", r.pkt.PacketType)
	}

	r = <-ch
	if r.err != nil {
		t.Fatal("unexpected error:", r.err)
	}

	if r.pkt.RequestID != id2 {
		t.Fatal("unexpected id in response:", r.pkt.RequestID)
	}

	switch r.pkt.PacketType {
	case sshfx.PacketTypeAttrs:
		// Second, we should get the LStat response: Attrs
		// We can go ahead and ignore this one.

	case sshfx.PacketTypeStatus:
		var status sshfx.StatusPacket

		if err := status.UnmarshalPacketBody(&r.pkt.Data); err != nil {
			t.Fatal("unexpected error:", err)
		}

		t.Fatal("unexpected status packet:", status)

	default:
		t.Fatal("unexpected type:", r.pkt.PacketType)
	}

	checkServerAllocator(t, server)
}

// Ensure that proper error codes are returned for non existent files, such
// that they are mapped back to a 'not exists' error on the client side.
func TestStatNonExistent(t *testing.T) {
	client, server := clientServerPair(t)
	defer client.Close()
	defer server.Close()

	for _, file := range []string{"/doesnotexist", "/doesnotexist/a/b"} {
		_, err := client.Stat(file)
		if !os.IsNotExist(err) {
			t.Errorf("expected 'does not exist' err for file %q.  got: %v", file, err)
		}
	}
}

func TestServerWithBrokenClient(t *testing.T) {
	validInit := sp(&sshFxInitPacket{Version: 3})
	brokenOpen := sp(&sshFxpOpenPacket{Path: "foo"})
	brokenOpen = brokenOpen[:len(brokenOpen)-2]

	for _, clientInput := range [][]byte{
		// Packet length zero (never valid). This used to crash the server.
		{0, 0, 0, 0},
		append(validInit, 0, 0, 0, 0),

		// Client hangs up mid-packet.
		append(validInit, brokenOpen...),
	} {
		srv, err := NewServer(struct {
			io.Reader
			io.WriteCloser
		}{
			bytes.NewReader(clientInput),
			&sink{},
		})
		require.NoError(t, err)

		err = srv.Serve()
		assert.Error(t, err)
		srv.Close()
	}
}
