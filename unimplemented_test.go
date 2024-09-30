package sftp

import (
	"testing"
)

func TestUnimplemented(t *testing.T) {
	type S struct {
		UnimplementedHandler
	}

	var _ ServerHandler = &S{}
}
