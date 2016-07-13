package sftp

import (
	"github.com/stretchr/testify/assert"

	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

type testHandler struct {
	filecontents string    // dummy contents
	output       io.Writer // dummy file out
	err          error     // dummy error, should be file related
}

func (t *testHandler) Fileread(r *Request) (io.Reader, error) {
	if t.err != nil { return nil, t.err }
	return strings.NewReader(t.filecontents), nil
}

func (t *testHandler) Filewrite(r *Request) (io.Writer, error) {
	if t.err != nil { return nil, t.err }
	return io.Writer(t.output), nil
}

func (t *testHandler) Filecmd(r *Request) error {
	if t.err != nil { return t.err }
	return nil
}

func (t *testHandler) Fileinfo(r *Request) ([]os.FileInfo, error) {
	if t.err != nil { return nil, t.err }
	f, err := os.Open(r.Filepath)
	if err != nil { return nil, err }
	fi, err := f.Stat()
	if err != nil { return nil, err }
	return []os.FileInfo{fi}, nil
}

func testRequest(method string) *Request {
	return &Request{
		Filepath: "./request_test.go",
		Method:   method,
		Pflags:   1,
		Attrs:    []byte("foo"),
		Target:   "foo",
		pkt_id:   1,
		data:     []byte("file-data."),
		length:   5,
	}
}

func newTestHandlers() Handlers {
	handler := &testHandler{
		filecontents: "file-data.",
		output:       bytes.NewBuffer([]byte{}),
		err:          nil,
	}
	return Handlers{
		FileGet:  handler,
		FilePut:  handler,
		FileCmd:  handler,
		FileInfo: handler,
	}
}

func (h Handlers) getOut() *bytes.Buffer {
	handler := h.FilePut.(*testHandler)
	return handler.output.(*bytes.Buffer)
}

var testError = errors.New("test error")

func (h *Handlers) returnError() {
	handler := h.FilePut.(*testHandler)
	handler.err = testError
}

func statusOk(t *testing.T, p interface{}) {
	if pkt, ok := p.(*sshFxpStatusPacket); ok {
		assert.Equal(t, pkt.id(), uint32(1))
		assert.Equal(t, pkt.StatusError.Code, uint32(ssh_FX_OK))
	}
}

func TestGetMethod(t *testing.T) {
	handlers := newTestHandlers()
	request := testRequest("Get")
	// req.length is 4, so we test reads in 4 byte chunks
	for _, txt := range []string{"file-", "data."} {
		pkt, err := request.handle(handlers)
		assert.Nil(t, err)
		dpkt := pkt.(*sshFxpDataPacket)
		assert.Equal(t, dpkt.id(), uint32(1))
		assert.Equal(t, string(dpkt.Data), txt)
	}
}

func TestPutMethod(t *testing.T) {
	handlers := newTestHandlers()
	request := testRequest("Put")
	pkt, err := request.handle(handlers)
	assert.Nil(t, err)
	assert.Equal(t, handlers.getOut().String(), "file-data.")
	statusOk(t, pkt)
}

func TestCmdrMethod(t *testing.T) {
	handlers := newTestHandlers()
	request := testRequest("Mkdir")
	pkt, err := request.handle(handlers)
	assert.Nil(t, err)
	statusOk(t, pkt)

	handlers.returnError()
	pkt, err = request.handle(handlers)
	assert.Nil(t, pkt)
	assert.Equal(t, err, testError)
}

func TestInfoListMethod(t *testing.T)     { testInfoMethod(t, "List") }
func TestInfoReadlinkMethod(t *testing.T) { testInfoMethod(t, "Readlink") }
func TestInfoStatMethod(t *testing.T) {
	handlers := newTestHandlers()
	request := testRequest("Stat")
	pkt, err := request.handle(handlers)
	assert.Nil(t, err)
	spkt := pkt.(*sshFxpStatResponse)
	assert.Equal(t, spkt.info.Name(), "request_test.go")
}

func testInfoMethod(t *testing.T, method string) {
	handlers := newTestHandlers()
	request := testRequest(method)
	pkt, err := request.handle(handlers)
	assert.Nil(t, err)
	npkt, ok := pkt.(*sshFxpNamePacket)
	assert.True(t, ok)
	assert.IsType(t, sshFxpNameAttr{}, npkt.NameAttrs[0])
	assert.Equal(t, npkt.NameAttrs[0].Name, "request_test.go")
}
