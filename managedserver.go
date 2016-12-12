package sftp

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"golang.org/x/crypto/ssh"
)

type LoginRequest struct {
	Username   string
	Password   string
	PublicKey  string
	RemoteAddr net.Addr
}

type ManagedServer struct {
	driverGenerator func(LoginRequest) ServerDriver
}

func NewManagedServer(driverGenerator func(LoginRequest) ServerDriver) *ManagedServer {
	return &ManagedServer{
		driverGenerator,
	}
}

func (m ManagedServer) Start(port int, rawPrivateKeys [][]byte) {
	log.Println("Starting SFTP server...")

	privateKeys := []ssh.Signer{}
	for i, rawKey := range rawPrivateKeys {
		privateKey, err := ssh.ParsePrivateKey(rawKey)
		if err != nil {
			log.Fatal("Failed to parse private key ", i, err)
		}
		privateKeys = append(privateKeys, privateKey)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", port))
	if err != nil {
		log.Fatal("failed to listen for connection", err)
	}
	log.Printf("Listening on %v\n", listener.Addr())

	for {
		newConn, err := listener.Accept()
		if err != nil {
			log.Fatal("failed to accept incoming connection", err)
		}

		go func(conn net.Conn) {
			var driver ServerDriver
			config := &ssh.ServerConfig{
				PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
					driver = m.driverGenerator(LoginRequest{
						Username:   c.User(),
						Password:   string(pass),
						PublicKey:  "",
						RemoteAddr: c.RemoteAddr(),
					})
					if driver == nil {
						return nil, fmt.Errorf("password rejected for %q", c.User())
					}
					return nil, nil
				},
				PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
					driver := m.driverGenerator(LoginRequest{
						Username:   c.User(),
						Password:   "",
						PublicKey:  strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))),
						RemoteAddr: c.RemoteAddr(),
					})
					if driver == nil {
						return nil, fmt.Errorf("password rejected for %q", c.User())
					}
					return nil, nil
				},
			}
			for _, privateKey := range privateKeys {
				config.AddHostKey(privateKey)
			}

			_, newChan, requestChan, err := ssh.NewServerConn(conn, config)
			if err != nil {
				if err != io.EOF {
					log.Println("failed to handshake", err)
				}
				return
			}
			log.Println("Handshake completed...")

			go ssh.DiscardRequests(requestChan)

			for newChannelRequest := range newChan {
				log.Println("Incoming channel: ", newChannelRequest.ChannelType())
				if newChannelRequest.ChannelType() != "session" {
					newChannelRequest.Reject(ssh.UnknownChannelType, "unknown channel type")
					log.Println("Unknown channel type:", newChannelRequest.ChannelType())
					continue
				}
				channel, requests, err := newChannelRequest.Accept()
				if err != nil {
					log.Println("could not accept channel", err)
				}
				log.Println("Channel accepted.")

				go func(in <-chan *ssh.Request) {
					for req := range in {
						log.Printf("Request: %v\n", req.Type)
						ok := false
						switch req.Type {
						case "subsystem":
							if len(req.Payload) >= 4 {
								log.Printf("Subsystem: %s\n", req.Payload[4:])
								if string(req.Payload[4:]) == "sftp" {
									ok = true
								}
							}
						}
						log.Printf(" - accepted: %v\n", ok)
						req.Reply(ok, nil)
					}
				}(requests)

				server, err := NewServer(channel, driver)

				if err != nil {
					log.Println("Error:", err)
					return
				}
				if err := server.Serve(); err != nil {
					log.Println("sftp server completed with error:", err)
					channel.Close()
				}
			}

		}(newConn)
	}
}
