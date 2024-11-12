package sftp

import (
	"testing"
)

func TestUnimplemented(t *testing.T) {
	type S struct {
		UnimplementedServerHandler
	}

	var _ ServerHandler = &S{}
}
