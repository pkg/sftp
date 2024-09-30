// streaming-read-benchmark benchmarks the peformance of reading
// from /dev/zero on the server to /dev/null on the client via io.Copy.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"github.com/pkg/sftp/v2"
)

var (
	User = flag.String("user", os.Getenv("USER"), "ssh username")
	Pass = flag.String("pass", os.Getenv("SOCKSIE_SSH_PASSWORD"), "ssh password")

	Host = flag.String("host", "localhost", "ssh server hostname")
	Port = flag.Int("port", 22, "ssh server port")

	Size = flag.Int("s", 1<<15, "set max packet size")
)

func init() {
	flag.Parse()
}

func main() {
	var auths []ssh.AuthMethod
	if aconn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err != nil {
		log.Fatal("unable to connect to auth agent:", err)
	} else {
		auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(aconn).Signers))
	}

	if *Pass != "" {
		auths = append(auths, ssh.Password(*Pass))
	}

	config := &ssh.ClientConfig{
		User:            *User,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", *Host, *Port)

	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		log.Fatalf("unable to connect to [%s]: %v", addr, err)
	}
	defer conn.Close()

	cl, err := sftp.NewClient(context.Background(), conn) // sftp.MaxPacket(*Size))
	if err != nil {
		log.Fatalf("unable to start sftp subsytem: %v", err)
	}
	defer cl.Close()

	r, err := cl.Open("/dev/zero")
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	w, err := os.OpenFile("/dev/null", os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer w.Close()

	const size int64 = 1e9

	log.Printf("reading %v bytes", size)

	t1 := time.Now()
	defer func() {
		log.Printf("read %v bytes in %s", size, time.Since(t1))
	}()

	n, err := io.Copy(w, io.LimitReader(r, size))
	if err != nil {
		log.Fatal(err)
	}

	if n != size {
		log.Fatalf("copy: expected %v bytes, got %d", size, n)
	}
}
