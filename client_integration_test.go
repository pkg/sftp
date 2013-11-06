package sftp

// sftp integration tests
// enable with -integration

import (
	"flag"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
)

var testIntegration = flag.Bool("integration", false, "perform integration tests against sftp server process")

// testClient returns a *Client connected to a localy running sftp-server
// the *exec.Cmd returned must be defer Wait'd.
func testClient(t *testing.T) (*Client, *exec.Cmd) {
	cmd := exec.Command("/usr/lib/openssh/sftp-server", "-e", "-R") // log to stderr, read only
	cmd.Stderr = os.Stdout
	pw, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	pr, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start sftp-server process: %v", err)
	}
	sftp := &Client{
		w: pw,
		r: pr,
	}
	if err := sftp.sendInit(); err != nil {
		defer cmd.Wait()
		t.Fatal(err)
	}
	if err := sftp.recvVersion(); err != nil {
		defer cmd.Wait()
		t.Fatal(err)
	}
	return sftp, cmd
}

func TestNewClient(t *testing.T) {
	sftp, cmd := testClient(t)
	defer cmd.Wait()

	if err := sftp.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientLstat(t *testing.T) {
	sftp, cmd := testClient(t)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	want, err := os.Lstat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	got, err := sftp.Lstat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	if !sameFile(want, got) {
		t.Fatalf("Lstat(%q): want %#v, got %#v", f.Name(), want, got)
	}
}

func TestClientOpen(t *testing.T) {
	sftp, cmd := testClient(t)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	got, err := sftp.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientRead(t *testing.T) {
	sftp, cmd := testClient(t)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString("Hello world!"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := sftp.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()

	b, err := ioutil.ReadAll(got)
	if err != nil {
		t.Fatal(err)
	}

	if want, got := "Hello world!", string(b); got != want {
		t.Fatalf("Read(): want %q, got %q", want, got)
	}
}

func TestClientFileStat(t *testing.T) {
	sftp, cmd := testClient(t)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	want, err := os.Lstat(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	f2, err := sftp.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	got, err := f2.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if !sameFile(want, got) {
		t.Fatalf("Lstat(%q): want %#v, got %#v", f.Name(), want, got)
	}
}

func sameFile(want, got os.FileInfo) bool {
	return want.Name() == got.Name() &&
		want.Size() == got.Size()
}
