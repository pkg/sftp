package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"code.google.com/p/go.crypto/ssh"

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
	if len(flag.Args()) < 1 {
		log.Fatal("subcommand required")
	}
}

func main() {
	var auths []ssh.ClientAuth
	if agent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK")); err == nil {
		auths = append(auths, ssh.ClientAuthAgent(ssh.NewAgentClient(agent)))
	}
	if *PASS != "" {
		auths = append(auths, ssh.ClientAuthPassword(password(*PASS)))
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

	client, err := sftp.NewClient(conn)
	if err != nil {
		log.Fatalf("unable to start sftp subsytem: %v", err)
	}
	defer client.Close()
	switch cmd := flag.Args()[0]; cmd {
	case "ls":
		if len(flag.Args()) < 2 {
			log.Fatalf("%s ls: remote path required", os.Args[0])
		}
		walker := client.Walk(flag.Args()[1])
		for walker.Step() {
			if err := walker.Err(); err != nil {
				log.Println(err)
				continue
			}
			fmt.Println(walker.Path())
		}
	case "fetch":
		if len(flag.Args()) < 2 {
			log.Fatalf("%s ls: remote path required", os.Args[0])
		}
		f, err := client.Open(flag.Args()[1])
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if _, err := io.Copy(os.Stdout, f); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("unknown subcommand: %v", cmd)
	}
}
