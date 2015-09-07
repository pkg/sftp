package sftp

import (
	"encoding/hex"
	"math/rand"
	"os"
	"testing"
	"time"
)

func randName() string {
	r := rand.New(rand.NewSource(time.Now().Unix()))
	data := make([]byte, 16)
	for i := 0; i < 16; i++ {
		data[i] = byte(r.Uint32())
	}
	return "sftp." + hex.EncodeToString(data)
}

func TestServerMkdirRmdir(t *testing.T) {
	listenerGo, hostGo, portGo := testServer(t, GOLANG_SFTP, READONLY)
	defer listenerGo.Close()

	tmpDir := "/tmp/" + randName()
	defer os.RemoveAll(tmpDir)

	// mkdir remote
	if _, err := runSftpClient(t, "mkdir "+tmpDir, "/", hostGo, portGo); err != nil {
		t.Fatal(err)
	}

	// directory should now exist
	if _, err := os.Stat(tmpDir); err != nil {
		t.Fatal(err)
	}

	// now remove the directory
	if _, err := runSftpClient(t, "rmdir "+tmpDir, "/", hostGo, portGo); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(tmpDir); err == nil {
		t.Fatal("should have error after deleting the directory")
	}
}

func TestServerSymlink(t *testing.T) {
	listenerGo, hostGo, portGo := testServer(t, GOLANG_SFTP, READONLY)
	defer listenerGo.Close()

	link := "/tmp/" + randName()
	defer os.RemoveAll(link)

	// now create a symbolic link within the new directory
	if output, err := runSftpClient(t, "symlink /bin/sh "+link, "/", hostGo, portGo); err != nil {
		t.Fatalf("failed: %v %v", err, string(output))
	}

	// symlink should now exist
	if stat, err := os.Lstat(link); err != nil {
		t.Fatal(err)
	} else if (stat.Mode() & os.ModeSymlink) != os.ModeSymlink {
		t.Fatalf("is not a symlink: %v", stat.Mode())
	}
}
