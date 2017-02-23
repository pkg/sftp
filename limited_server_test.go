package sftp

import (
	"errors"
	"io"
	"io/ioutil"
	"os"
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

	notifyChan := make(chan string)
	uploadNotifier := func(name string) {
		notifyChan <- name
	}

	serverOptions := []ServerOption{
		UploadPath(uploadPath),
		FileNameMapper(fileNameMapper),
		UploadNotifier(uploadNotifier),
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
}
