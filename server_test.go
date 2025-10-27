package sftp

import (
	"testing"
)

func TestNonExistentHandle(t *testing.T) {
	type S struct {
		UnimplementedServerHandler
	}

	srv := Server{
		Handler: &S{},
	}

	if _, err := srv.FileFromHandle("noexist"); err == nil {
		t.Errorf("expected an error from FileFromHandle(%q)", "noexist")
	}

	if _, err := srv.DirFromHandle("noexist"); err == nil {
		t.Errorf("expected an error from FileFromHandle(%q)", "noexist")
	}
}
