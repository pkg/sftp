// An example SFTP server implementation using the golang SSH package.
// Serves the whole filesystem visible to the user, and has a hard-coded username and password.
// DO NOT USE FOR A REAL SYSTEM!
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/pkg/sftp/v2"
	"github.com/pkg/sftp/v2/localfs"
	"golang.org/x/crypto/ssh"
)

var (
	readOnly = flag.Bool("read-only", false, "read-only server")
	debugStderr = flag.Bool("debug", false, "debug to stderr")
)

// Based on example server code from golang.org/x/crypto/ssh and server_standalone
func main() {
	flag.Parse()

	debug := io.Discard
	if *debugStderr {
		debug = os.Stderr
	}

	// An SSH server is represented by a ServerConfig, which holds
	// certificate details and handles authentication of ServerConns.
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			// Should use constant-time compare (or better, salt+hash) in
			// a production setting.
			fmt.Fprintln(debug, "Login:", c.User())
			if c.User() == "testuser" && string(pass) == "hunter2" {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected for %q", c.User())
		},
	}

	privateBytes, err := os.ReadFile("id_rsa")
	if err != nil {
		log.Fatal("Failed to load private key", err)
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatal("Failed to parse private key", err)
	}

	config.AddHostKey(private)

	// Once a ServerConfig has been configured, connections can be
	// accepted.
	l, err := net.Listen("tcp", ":2022")
	if err != nil {
		log.Fatal("failed to listen on port 2022:", err)
	}

	fmt.Println("Listening on", l.Addr())

	conn, err := l.Accept()
	if err != nil {
		log.Fatal("failed to accept incoming connection:", err)
	}

	// Before use, a handshake must be performed on the incoming net.Conn.
	_, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		log.Fatal("failed ssh handshake:", err)
	}

	fmt.Fprintln(debug, "SSH server established")

	// The incoming Request channel must be serviced.
	go ssh.DiscardRequests(reqs)

	// Service the incoming Channel channel.
	for newCh := range chans {
		// Channels have a type, depending on the application level protocol intended.
		// In the case of an SFTP session, this is "subsystem" with a payload string of "<length=4>sftp"
		fmt.Fprintln(debug, "Incoming channel:", newCh.ChannelType())

		if newCh.ChannelType() != "session" {
			newCh.Reject(ssh.UnknownChannelType, "unknown channel type")
			fmt.Fprintln(debug, "Unknown channel type:", newCh.ChannelType())
			continue
		}

		conn, req, err := newCh.Accept()
		if err != nil {
			log.Fatal("could not accept channel:", err)
		}

		fmt.Fprintln(debug, "Channel accepted")

		// Sessions have out-of-band requests such as "shell",
		// "pty-req" and "env".  Here we handle only the
		// "subsystem" request.
		go func() {
			for req := range req {
				fmt.Fprintln(debug, "Request:", req.Type)

				ok := false

				switch req.Type {
				case "subsystem":
					fmt.Fprintln(debug, "Subsystem:", string(req.Payload[4:]))
					if string(req.Payload[4:]) == "sftp" {
						ok = true
					}
				}

				fmt.Fprintln(debug, " - accepted:", ok)
				req.Reply(ok, nil)
			}
		}()

		if *readOnly {
			fmt.Fprintln(debug, "Read-only server")
		} else {
			fmt.Fprintln(debug, "Read/write server")
		}

		srv := &sftp.Server{
			Handler:  &localfs.ServerHandler{
				ReadOnly: *readOnly,
			},
			Debug: debug,
		}

		if err := srv.Serve(conn); err != nil {
			log.Fatal("sftp server completed with error:", err)
		}

		srv.GracefulStop()
		log.Print("sftp client exited session.")
	}
}
