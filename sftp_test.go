package sftp

import (
	"io"
	"syscall"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestErrFxCode(t *testing.T) {
	ider := sshFxpStatusPacket{ID: 1}
	table := []struct {
		err error
		fx  fxerr
	}{
		{err: errors.New("random error"), fx: ErrSSHFxFailure},
		{err: syscall.EBADF, fx: ErrSSHFxFailure},
		{err: syscall.ENOENT, fx: ErrSSHFxNoSuchFile},
		{err: syscall.EPERM, fx: ErrSSHFxPermissionDenied},
		{err: io.EOF, fx: ErrSSHFxEOF},
	}
	for _, tt := range table {
		statusErr := statusFromError(ider, tt.err).StatusError
		assert.Equal(t, statusErr.FxCode(), tt.fx)
	}
}
