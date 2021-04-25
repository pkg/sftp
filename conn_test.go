package sftp

import (
	"errors"
	"io"
	"os"
	"testing"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

func TestStatusToError(t *testing.T) {
	var tests = []struct {
		desc string
		code sshfx.Status
		want error
	}{
		{
			desc: "SSH_FX_OK",
			code: sshfx.StatusOK,
		},
		{
			desc: "SSH_FX_EOF",
			code: sshfx.StatusEOF,
			want: io.EOF,
		},
		{
			desc: "SSH_FX_NO_SUCH_FILE",
			code: sshfx.StatusNoSuchFile,
			want: os.ErrNotExist,
		},
		{
			desc: "SSH_FX_PERMISSION_DENIED",
			code: sshfx.StatusPermissionDenied,
			want: os.ErrPermission,
		},
		{
			desc: "SSH_FX_FAILURE",
			code: sshfx.StatusFailure,
			want: &StatusError{Code: uint32(sshfx.StatusFailure)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			pkt := &sshfx.StatusPacket{
				StatusCode: tt.code,
			}

			if got := statusToError(pkt); !errors.Is(got, tt.want) {
				t.Errorf("statusToError(%s), = %#v, want: %#v", tt.code, got, tt.want)
			}
		})
	}
}
