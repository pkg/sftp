package sftp

import (
	"sync"

	"github.com/stretchr/testify/assert"

	"bytes"
	"errors"
	"io"
	"os"
	"testing"
)

type testHandler struct {
	filecontents []byte      // dummy contents
	output       io.WriterAt // dummy file out
	err          error       // dummy error, should be file related
}

func (t *testHandler) Fileread(r *Request) (io.ReaderAt, error) {
	if t.err != nil {
		return nil, t.err
	}
	return bytes.NewReader(t.filecontents), nil
}

func (t *testHandler) Filewrite(r *Request) (io.WriterAt, error) {
	if t.err != nil {
		return nil, t.err
	}
	return io.WriterAt(t.output), nil
}

func (t *testHandler) Filecmd(r *Request) error {
	return t.err
}

func (t *testHandler) Filelist(r *Request) (ListerAt, error) {
	if t.err != nil {
		return nil, t.err
	}
	f, err := os.Open(r.Filepath)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return listerat([]os.FileInfo{fi}), nil
}

// make sure len(fakefile) == len(filecontents)
type fakefile [10]byte

var filecontents = []byte("file-data.")

func testRequest(method string) *Request {
	request := &Request{
		Filepath:  "./request_test.go",
		Method:    method,
		Attrs:     []byte("foo"),
		Target:    "foo",
		packets:   make(chan packet_data, SftpServerWorkerCount),
		state:     &state{},
		stateLock: &sync.RWMutex{},
	}
	for _, p := range []packet_data{
		{_id: 1, data: filecontents[:5], length: 5},
		{_id: 2, data: filecontents[5:], length: 5, offset: 5}} {
		request.packets <- p
	}
	return request
}

func (ff *fakefile) WriteAt(p []byte, off int64) (int, error) {
	n := copy(ff[off:], p)
	return n, nil
}

func (ff fakefile) string() string {
	b := make([]byte, len(ff))
	copy(b, ff[:])
	return string(b)
}

func newTestHandlers() Handlers {
	handler := &testHandler{
		filecontents: filecontents,
		output:       &fakefile{},
		err:          nil,
	}
	return Handlers{
		FileGet:  handler,
		FilePut:  handler,
		FileCmd:  handler,
		FileList: handler,
	}
}

func (h Handlers) getOutString() string {
	handler := h.FilePut.(*testHandler)
	return handler.output.(*fakefile).string()
}

var errTest = errors.New("test error")

func (h *Handlers) returnError() {
	handler := h.FilePut.(*testHandler)
	handler.err = errTest
}

func statusOk(t *testing.T, p interface{}) {
	if pkt, ok := p.(*sshFxpStatusPacket); ok {
		assert.Equal(t, pkt.StatusError.Code, uint32(ssh_FX_OK))
	}
}

func TestRequestGet(t *testing.T) {
	handlers := newTestHandlers()
	request := testRequest("Get")
	// req.length is 5, so we test reads in 5 byte chunks
	for i, txt := range []string{"file-", "data."} {
		pkt := request.callHandler(handlers)
		dpkt := pkt.(*sshFxpDataPacket)
		assert.Equal(t, dpkt.id(), uint32(i+1))
		assert.Equal(t, string(dpkt.Data), txt)
	}
}

func TestRequestPut(t *testing.T) {
	handlers := newTestHandlers()
	request := testRequest("Put")
	pkt := request.callHandler(handlers)
	statusOk(t, pkt)
	pkt = request.callHandler(handlers)
	statusOk(t, pkt)
	assert.Equal(t, "file-data.", handlers.getOutString())
}

func TestRequestCmdr(t *testing.T) {
	handlers := newTestHandlers()
	request := testRequest("Mkdir")
	pkt := request.callHandler(handlers)
	statusOk(t, pkt)

	handlers.returnError()
	pkt = request.callHandler(handlers)
	assert.Equal(t, pkt, statusFromError(pkt, errTest))
}

func TestRequestInfoList(t *testing.T)     { testInfoMethod(t, "List") }
func TestRequestInfoReadlink(t *testing.T) { testInfoMethod(t, "Readlink") }
func TestRequestInfoStat(t *testing.T) {
	handlers := newTestHandlers()
	request := testRequest("Stat")
	pkt := request.callHandler(handlers)
	spkt, ok := pkt.(*sshFxpStatResponse)
	assert.True(t, ok)
	assert.Equal(t, spkt.info.Name(), "request_test.go")
}

func testInfoMethod(t *testing.T, method string) {
	handlers := newTestHandlers()
	request := testRequest(method)
	pkt := request.callHandler(handlers)
	npkt, ok := pkt.(*sshFxpNamePacket)
	assert.True(t, ok)
	assert.IsType(t, sshFxpNameAttr{}, npkt.NameAttrs[0])
	assert.Equal(t, npkt.NameAttrs[0].Name, "request_test.go")
}
