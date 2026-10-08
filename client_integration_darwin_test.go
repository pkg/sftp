package sftp

import (
	"syscall"
	"testing"
)

func TestClientStatVFS(t *testing.T) {
	if *testServerImpl {
		t.Skipf("go server does not support FXP_EXTENDED")
	}
	sftp, cmd := testClient(t, READWRITE, NODELAY)
	defer cmd.Wait()
	defer sftp.Close()

	if _, ok := sftp.HasExtension("statvfs@openssh.com"); !ok {
		t.Fatal("server doesn't list statvfs extension")
	}

	vfs, err := sftp.StatVFS("/")
	if err != nil {
		t.Fatal(err)
	}

	// get system stats
	s := syscall.Statfs_t{}
	err = syscall.Statfs("/", &s)
	if err != nil {
		t.Fatal(err)
	}

	if vfs.Frsize != uint64(s.Bsize) {
		t.Fatalf("f_frsize does not match, expected: %v, got: %v", s.Bsize, vfs.Frsize)
	}

	if vfs.Bsize != uint64(s.Iosize) {
		t.Fatalf("f_bsize does not match, expected: %v, got: %v", s.Iosize, vfs.Bsize)
	}

	if vfs.Blocks != s.Blocks {
		t.Fatalf("f_blocks does not match, expected: %v, got: %v", s.Blocks, vfs.Blocks)
	}
}
