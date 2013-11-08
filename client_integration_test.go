package sftp

// sftp integration tests
// enable with -integration

import (
	"flag"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"testing"
)

const (
	READONLY  = true
	READWRITE = false

	debuglevel = "DEBUG"
)

var testIntegration = flag.Bool("integration", false, "perform integration tests against sftp server process")

// testClient returns a *Client connected to a localy running sftp-server
// the *exec.Cmd returned must be defer Wait'd.
func testClient(t *testing.T, readonly bool) (*Client, *exec.Cmd) {
	if !*testIntegration {
		t.Skip("skipping intergration test")
	}
	cmd := exec.Command("/usr/lib/openssh/sftp-server", "-e", "-R", "-l", debuglevel) // log to stderr, read only
	if !readonly {
		cmd = exec.Command("/usr/lib/openssh/sftp-server", "-e", "-l", debuglevel) // log to stderr
	}
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
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()

	if err := sftp.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientLstat(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
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

func TestClientLstatiMissing(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(f.Name())

	_, err = sftp.Lstat(f.Name())
	if err1, ok := err.(*StatusError); !ok || err1.Code != ssh_FX_NO_SUCH_FILE {
		t.Fatalf("Lstat: want: %v, got %#v", ssh_FX_NO_SUCH_FILE, err)
	}
}

func TestClientOpen(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
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
	sftp, cmd := testClient(t, READONLY)
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

func TestClientCreate(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	f2, err := sftp.Create(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
}

func TestClientCreateFailed(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	f, err := ioutil.TempFile("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	f2, err := sftp.Create(f.Name())
	if err1, ok := err.(*StatusError); !ok || err1.Code != ssh_FX_PERMISSION_DENIED {
		t.Fatalf("Create: want: %v, got %#v", ssh_FX_PERMISSION_DENIED, err)
	}
	if err == nil {
		f2.Close()
	}
}

func TestClientFileStat(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
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

func TestClientWalkdir(t *testing.T) {
	sftp, cmd := testClient(t, READONLY)
	defer cmd.Wait()
	defer sftp.Close()

	d, err := ioutil.TempDir("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(d)

	w := sftp.Walk(d)
	for w.Step() {
		if err := w.Err(); err != nil {
			t.Error(err)
			continue
		}
		t.Log(w.Path())
	}
}

func sameFile(want, got os.FileInfo) bool {
	return want.Name() == got.Name() &&
		want.Size() == got.Size()
}

var clientWriteTests = []struct {
	n     int
	total int64 // cumulative file size
}{
	{0, 0},
	{1, 1},
	{0, 1},
	{999, 1000},
	{24, 1024},
	{1023, 2047},
	{2048, 4095},
	{1 << 12, 8191},
	{1 << 13, 16383},
	{1 << 14, 32767},
	{1 << 15, 65535},
	{1 << 16, 131071},
	{1 << 17, 262143},
	// TODO(dfc) too large
	// 	{1 << 18, 262143},
}

func TestClientWrite(t *testing.T) {
	sftp, cmd := testClient(t, READWRITE)
	defer cmd.Wait()
	defer sftp.Close()

	d, err := ioutil.TempDir("", "sftptest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(d)

	f := path.Join(d, "writeTest")
	w, err := sftp.Create(f)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	for _, tt := range clientWriteTests {
		got, err := w.Write(make([]byte, tt.n))
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.n {
			t.Errorf("Write(%v): wrote: want: %v, got %v", tt.n, tt.n, got)
		}
		fi, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if total := fi.Size(); total != tt.total {
			t.Errorf("Write(%v): size: want: %v, got %v", tt.n, tt.total, total)
		}
	}
}
