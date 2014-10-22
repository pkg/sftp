// gsftp implements a simple sftp client.
//
// gsftp understands the following commands:
//
// List a directory (and its subdirectories)
//      gsftp ls DIR
//
// Fetch a remote file
//      gsftp fetch FILE
//
// Put the contents of stdin to a remote file
//      cat LOCALFILE | gsftp put REMOTEFILE
//
// Print the details of a remote file
//      gsftp stat FILE
//
// Remove a remote file
//      gsftp rm FILE
//
// Rename a file
//      gsftp mv OLD NEW
//
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"

	"code.google.com/p/go.crypto/ssh"
	"code.google.com/p/go.crypto/ssh/agent"

	"github.com/pkg/sftp"
)

var (
	USER = flag.String("user", os.Getenv("USER"), "ssh username")
	HOST = flag.String("host", "localhost", "ssh server hostname")
	PORT = flag.Int("port", 22, "ssh server port")
	PASS = flag.String("pass", os.Getenv("SOCKSIE_SSH_PASSWORD"), "ssh password")
)

func init() {
	flag.Parse()
}

func main() {
	var auths []ssh.AuthMethod
	if aconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(aconn).Signers))

	}
	if *PASS != "" {
		auths = append(auths, ssh.Password(*PASS))
	}

	config := ssh.ClientConfig{
		User: *USER,
		Auth: auths,
	}
	addr := fmt.Sprintf("%s:%d", *HOST, *PORT)
	conn, err := ssh.Dial("tcp", addr, &config)
	if err != nil {
		log.Fatalf("unable to connect to [%s]: %v", addr, err)
	}
	defer conn.Close()

	c, err := sftp.NewClient(conn)
	if err != nil {
		log.Fatalf("unable to start sftp subsytem: %v", err)
	}
	defer c.Close()

	rName := "/tmp/zero.img"

	fi, err := c.Lstat(rName)
	if err != nil {
		log.Fatal(err)
	}

	fp, err := c.Open(rName)
	if err != nil {
		log.Fatal(err)
	}
	defer fp.Close()

	n, err := io.Copy(ioutil.Discard, fp)
	if err != nil {
		log.Fatal(err)
	}
	if n != fi.Size() {
		log.Fatalf("copy %d != %d remote size", n, fi.Size())
	}
}
