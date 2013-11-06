package sftp

import "io"
import "testing"

var ok = &StatusError{Code: SSH_FX_OK}
var eof = &StatusError{Code: SSH_FX_EOF}
var fail = &StatusError{Code: SSH_FX_FAILURE}

var eofOrErrTests = []struct {
	err, want error
}{
	{nil, nil},
	{eof, io.EOF},
	{ok, ok},
	{io.EOF, io.EOF},
}

func TestEofOrErr(t *testing.T) {
	for _, tt := range eofOrErrTests {
		got := eofOrErr(tt.err)
		if got != tt.want {
			t.Errorf("eofOrErr(%#v): want: %#v, got: %#v", tt.err, tt.want, got)
		}
	}
}

var okOrErrTests = []struct {
	err, want error
}{
	{nil, nil},
	{eof, eof},
	{ok, nil},
	{io.EOF, io.EOF},
}

func TestOkOrErr(t *testing.T) {
	for _, tt := range okOrErrTests {
		got := okOrErr(tt.err)
		if got != tt.want {
			t.Errorf("okOrErr(%#v): want: %#v, got: %#v", tt.err, tt.want, got)
		}
	}
}
