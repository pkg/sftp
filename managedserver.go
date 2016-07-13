package sftp

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"

	"golang.org/x/crypto/ssh"
)

type LoginRequest struct {
	Username string
	Password string
}

type ManagedServer struct {
	driverGenerator func(LoginRequest) ServerDriver
}

func NewManagedServer(driverGenerator func(LoginRequest) ServerDriver) *ManagedServer {
	return &ManagedServer{
		driverGenerator,
	}
}

func (m ManagedServer) Start(port int, privateKeyPath string) {
	fmt.Println("Starting SFTP server...")

	privateBytes, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		log.Fatal("Failed to load private key", err)
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		log.Fatal("Failed to parse private key", err)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", port))
	if err != nil {
		log.Fatal("failed to listen for connection", err)
	}
	fmt.Printf("Listening on %v\n", listener.Addr())

	for {
		newConn, err := listener.Accept()
		if err != nil {
			log.Fatal("failed to accept incoming connection", err)
		}

		go func(conn net.Conn) {
			fmt.Println("Got connection!")

			var driver ServerDriver
			config := &ssh.ServerConfig{
				PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
					driver = m.driverGenerator(LoginRequest{
						Username: c.User(),
						Password: string(pass),
					})
					if driver == nil {
						return nil, fmt.Errorf("password rejected for %q", c.User())
					}
					return nil, nil
				},
			}
			config.AddHostKey(private)

			_, newChan, requestChan, err := ssh.NewServerConn(conn, config)
			if err != nil {
				log.Fatal("failed to handshake", err)
			}
			fmt.Println("Handshake completed...")

			go ssh.DiscardRequests(requestChan)

			for newThing := range newChan {
				fmt.Println("Incoming channel: ", newThing.ChannelType())
				if newThing.ChannelType() != "session" {
					newThing.Reject(ssh.UnknownChannelType, "unknown channel type")
					fmt.Println("Unknown channel type:", newThing.ChannelType())
					continue
				}
				channel, requests, err := newThing.Accept()
				if err != nil {
					fmt.Println("could not accept channel", err)
				}
				fmt.Println("Channel accepted.")

				go func(in <-chan *ssh.Request) {
					for req := range in {
						fmt.Printf("Request: %v\n", req.Type)
						ok := false
						switch req.Type {
						case "subsystem":
							fmt.Printf("Subsystem: %s\n", req.Payload[4:])
							if string(req.Payload[4:]) == "sftp" {
								ok = true
							}
						}
						fmt.Printf(" - accepted: %v\n", ok)
						req.Reply(ok, nil)
					}
				}(requests)

				server, err := NewServer(channel, driver)

				if err != nil {
					fmt.Println("Error:", err)
				}
				if err := server.Serve(); err != nil {
					fmt.Println("sftp server completed with error:", err)
					channel.Close()
				}
			}

		}(newConn)
	}
}
