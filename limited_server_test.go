package sftp

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"
)

func TestLimitedServer(t *testing.T) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()

	uploadDir, err := ioutil.TempDir("", "limited_sftp_test_")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(uploadDir)

	uploadPath := "/unvisioned/mockernut"

	fileNameMapper := func(name string) (string, bool, error) {
		switch name[0] {
		case 'E':
			return "", false, errors.New("server error")
		case 'A':
		default:
			return "", false, nil
		}
		return uploadDir + "/" + name, true, nil
	}

	fileList := []os.FileInfo{
		&fileInfo{name: "moon-pie", size: 12345},
		&fileInfo{name: "toaster", size: 42},
		&fileInfo{name: "woof", size: 9999},
	}
	var fileListPos int

	notifyChan := make(chan string)
	uploadNotifier := func(name string) {
		notifyChan <- name
	}

	opendirHook := func() {
		fileListPos = 0
	}

	readdirHook := func() (string, []os.FileInfo, error) {
		if fileListPos >= len(fileList) {
			return "", nil, io.EOF
		} else {
			start := fileListPos
			fileListPos = len(fileList)
			return uploadPath, fileList[start:], nil
		}
	}

	serverOptions := []ServerOption{
		UploadPath(uploadPath),
		FileNameMapper(fileNameMapper),
		UploadNotifier(uploadNotifier),
		OpendirHook(opendirHook),
		ReaddirHook(readdirHook),
	}

	server, err := NewServer(struct {
		io.Reader
		io.WriteCloser
	}{sr, sw}, serverOptions...)
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve()

	client, err := NewClientPipe(cr, cw)
	if err != nil {
		t.Fatalf("%+v\n", err)
	}

	wd, err := client.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if wd != uploadPath {
		t.Errorf("Expected %q, got %q", uploadPath, wd)
	}

	fileName := "Aphellogen-weatherer"
	fileContent := "periodate-tritanopia"

	f, err := client.Create(uploadPath + "/" + fileName)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write([]byte(fileContent))
	if err != nil {
		t.Fatal(err)
	}

	// Reading is not allowed.
	_, err = f.Read(make([]byte, 32))
	if err == nil {
		t.Error("Read didn't fail")
	}

	err = f.Close()
	if err != nil {
		t.Fatal(err)
	}

	expectedFileName := uploadDir + "/" + fileName
	select {
	case fn := <-notifyChan:
		if fn != expectedFileName {
			t.Errorf("Expected %q, got %q", expectedFileName, fn)
		}
	case <-time.After(time.Second):
		t.Fatal("Timed out")
	}

	fc, err := ioutil.ReadFile(expectedFileName)
	if err != nil {
		t.Fatal(err)
	}
	if string(fc) != fileContent {
		t.Errorf("Expected %q, got %q", fileContent, string(fc))
	}

	// Try readdir. Do it twice to make sure directory is reset for the 2nd time.
	for i := 0; i < 2; i++ {
		list, err := client.ReadDir(uploadPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != len(fileList) {
			t.Errorf("Wrong number of files; expect %d, got %d\n", len(fileList), len(list))
		} else {
			for i, f := range list {
				if f.Name() != fileList[i].Name() {
					t.Error("File %d wrong name\n", i)
				}
				if f.Size() != fileList[i].Size() {
					t.Error("File %d wrong size\n", i)
				}
			}
		}
	}

	// Try lstat.
	fi, err := client.Lstat(uploadPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Name() != path.Base(uploadPath) {
		t.Errorf("Lstat returned wrong file name %q", fi.Name())
	}

	// Try setstat. This is a no-op.
	err = client.Chmod("rederivation-nubiferous", 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Try failure conditions.

	// Invalid file name
	_, err = client.Create(uploadPath + "/forfare-semicrome")
	if err == nil {
		t.Error("Invalid file name didn't fail")
	}

	// Simulate server error.
	_, err = client.Create(uploadPath + "/Edextrinous-serpulae")
	if err == nil {
		t.Error("No server failure")
	}

	// Bad directory
	_, err = client.Create("/aerobus-veillike/unexpandable-perturbedly")
	if err == nil {
		t.Error("Bad directory didn't fail")
	}

	// Not directly under upload path
	_, err = client.Create(uploadPath + "/cohibit-perinephrial/tuberoid-schiffli")
	if err == nil {
		t.Error("Not directly under upload path didn't fail")
	}

	// Read-only
	_, err = client.Open(uploadPath + "/" + "Alexure-perviously")
	if err == nil {
		t.Error("Read-only open didn't fail")
	}

	// Unsupported
	err = client.Mkdir(uploadPath + "/" + "deviationist-arborous")
	if err == nil {
		t.Error("Mkdir didn't fail")
	}

	// Readdir of directory other than uploadPath.
	_, err = client.ReadDir(path.Dir(uploadPath))
	if err == nil {
		t.Error("Bad readdir didn't fail")
	}

	// Lstat of directory other than uploadPath.
	_, err = client.Lstat(uploadPath + "/..")
	if err == nil {
		t.Error("Bad lstat didn't fail")
	}
}
