package sftp_test

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func Example() {
	var conn *ssh.Client

	// open an SFTP session over an existing ssh connection.
	client, err := sftp.NewClient(conn)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// walk a directory
	w := client.Walk("/home/user")
	for w.Step() {
		if w.Err() != nil {
			continue
		}
		log.Println(w.Path())
	}

	// leave your mark
	f, err := client.Create("hello.txt")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := f.Write([]byte("Hello world!")); err != nil {
		log.Fatal(err)
	}
	f.Close()

	// check it's there
	fi, err := client.Lstat("hello.txt")
	if err != nil {
		log.Fatal(err)
	}
	log.Println(fi)
}

func ExampleNewClientPipe() {
	// Connect to a remote host and request the sftp subsystem via the 'ssh'
	// command.  This assumes that passwordless login is correctly configured.
	cmd := exec.Command("ssh", "example.com", "-s", "sftp")

	// send errors from ssh to stderr
	cmd.Stderr = os.Stderr

	// get stdin and stdout
	wr, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	rd, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal(err)
	}

	// start the process
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	defer cmd.Wait()

	// open the SFTP session
	client, err := sftp.NewClientPipe(rd, wr)
	if err != nil {
		log.Fatal(err)
	}

	// read a directory
	list, err := client.ReadDir("/")
	if err != nil {
		log.Fatal(err)
	}

	// print contents
	for _, item := range list {
		fmt.Println(item.Name())
	}

	// close the connection
	client.Close()
}

func ExampleClient_Mkdir_parents() {
	// Example of mimicing 'mkdir --parents'; I.E. recursively create
	// directoryies and don't error if any directories already exists.
	var conn *ssh.Client

	client, err := sftp.NewClient(conn)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	sshFxFailure := uint32(4)
	mkdirParents := func(client *sftp.Client, dir string) (err error) {
		var parents string

		if path.IsAbs(dir) {
			// Otherwise, an absolute path given below would be turned in to a relative one
			// by splitting on "/"
			parents = "/"
		}

		for _, name := range strings.Split(dir, "/") {
			if name == "" {
				// Paths with double-/ in them should just move along
				// this will also catch the case of the first character being a "/", i.e. an absolute path
				continue
			}
			parents = path.Join(parents, name)
			err = client.Mkdir(parents)
			if status, ok := err.(*sftp.StatusError); ok {
				if status.Code == sshFxFailure {
					var fi os.FileInfo
					fi, err = client.Stat(parents)
					if err == nil {
						if !fi.IsDir() {
							return fmt.Errorf("file exists: %s", parents)
						}
					}
				}
			}
			if err != nil {
				break
			}
		}
		return err
	}

	err = mkdirParents(client, "/tmp/foo/bar")
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleFile_ReadFrom_bufio() {
	// Using Bufio to buffer writes going to an sftp.File won't buffer as it
	// skips buffering if the underlying writer support ReadFrom. The
	// workaround is to wrap your writer in a struct that only implements
	// io.Writer.
	//
	// For background see github.com/pkg/sftp/issues/125

	var data_source io.Reader
	var f *sftp.File
	type writerOnly struct{ io.Writer }
	bw := bufio.NewWriter(writerOnly{f}) // no ReadFrom()
	bw.ReadFrom(data_source)
}

func ExampleNewClientPipe_tls() {
	var clientCert tls.Certificate

	// SFTP does not need SSH underneath: any reliable stream will do, as long
	// as the other end runs an SFTP server on it (see ExampleNewServer).
	// A plain TCP connection works the same way, but has no encryption or
	// authentication at all, so use TLS and authenticate the client, here
	// with a client certificate.
	conn, err := tls.Dial("tcp", "example.com:2022", &tls.Config{
		Certificates: []tls.Certificate{clientCert},
	})
	if err != nil {
		log.Fatal(err)
	}

	// open the SFTP session
	client, err := sftp.NewClientPipe(conn, conn)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// read a directory
	list, err := client.ReadDir("/")
	if err != nil {
		log.Fatal(err)
	}

	// print contents
	for _, item := range list {
		fmt.Println(item.Name())
	}
}

func ExampleNewServer() {
	var serverCert tls.Certificate
	var clientCAs *x509.CertPool

	// Serve SFTP directly over TLS, without SSH. SFTP has no authentication
	// of its own, so require and verify a client certificate.
	listener, err := tls.Listen("tcp", ":2022", &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
	})
	if err != nil {
		log.Fatal(err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}

		// NewServer gives every client the local filesystem with the
		// permissions of the user this process runs as.
		// Use NewRequestServer to control what clients can do.
		go func() {
			server, err := sftp.NewServer(conn)
			if err != nil {
				log.Print(err)
				conn.Close()
				return
			}
			if err := server.Serve(); err != nil && err != io.EOF {
				log.Print("sftp server completed with error: ", err)
			}
			conn.Close()
		}()
	}
}
