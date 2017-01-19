package sftp

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"golang.org/x/crypto/ssh"
)

// Logger is an abstraction for how logging will be performed by the server. It matches
// a subset of the Clever/kayvee-go library.
type Logger interface {
	InfoD(title string, meta map[string]interface{})
	ErrorD(title string, meta map[string]interface{})
}

// meta is a shorthand for map[string]interface{} to make logger calls more concise.
type meta map[string]interface{}

// LoginRequest is the metadata associated with a login request that is passed to the
// driverGenerator function in order for it to approve/deny the request.
type LoginRequest struct {
	Username   string
	Password   string
	PublicKey  string
	RemoteAddr net.Addr
}

// ManagedServer is our term for the SFTP server.
type ManagedServer struct {
	driverGenerator func(LoginRequest) ServerDriver
	lg              Logger
}

// NewManagedServer creates a new ManagedServer which conditionally serves requests based
// on the output of driverGenerator.
func NewManagedServer(driverGenerator func(LoginRequest) ServerDriver, lg Logger) *ManagedServer {
	return &ManagedServer{
		driverGenerator: driverGenerator,
		lg:              lg,
	}
}

// Start actually starts the server and begins fielding requests.
func (m ManagedServer) Start(port int, rawPrivateKeys [][]byte, ciphers, macs []string) {
	m.lg.InfoD("starting-server", meta{
		"port":    port,
		"ciphers": ciphers,
		"macs":    macs,
	})

	privateKeys := []ssh.Signer{}
	for i, rawKey := range rawPrivateKeys {
		privateKey, err := ssh.ParsePrivateKey(rawKey)
		if err != nil {
			m.lg.ErrorD("private-key-parse", meta{"index": i, "error": err.Error()})
			os.Exit(1)
		}
		privateKeys = append(privateKeys, privateKey)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", port))
	if err != nil {
		m.lg.ErrorD("listen-fail", meta{
			"msg":   "failed to open socket",
			"error": err.Error(),
			"port":  port})
	}
	m.lg.InfoD("listening", meta{"address": listener.Addr().String()})

	for {
		newConn, err := listener.Accept()
		if err != nil {
			m.lg.ErrorD("listener-accept-fail", meta{"error": err.Error()})
			os.Exit(1)
		}

		go func(conn net.Conn) {
			var driver ServerDriver
			config := &ssh.ServerConfig{
				Config: ssh.Config{
					Ciphers: ciphers,
					MACs:    macs,
				},
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
					m.lg.ErrorD("handshake-failure", meta{"error": err.Error()})
				}
				return
			}
			m.lg.InfoD("handshake-complete", meta{})

			go ssh.DiscardRequests(requestChan)

			for newChannelRequest := range newChan {
				m.lg.InfoD("incoming-channel", meta{"type": newChannelRequest.ChannelType()})
				if newChannelRequest.ChannelType() != "session" {
					newChannelRequest.Reject(ssh.UnknownChannelType, "unknown channel type")
					m.lg.ErrorD("unknown-channel-type", meta{"type": newChannelRequest.ChannelType()})
					continue
				}
				channel, requests, err := newChannelRequest.Accept()
				if err != nil {
					m.lg.ErrorD("channel-accept-failure", meta{
						"err":  err.Error(),
						"type": newChannelRequest.ChannelType()})
					return
				}
				m.lg.ErrorD("channel-accepted", meta{})

				go func(in <-chan *ssh.Request) {
					for req := range in {
						m.lg.ErrorD("ssh-request", meta{"type": req.Type})
						ok := false
						switch req.Type {
						case "subsystem":
							if len(req.Payload) >= 4 {
								m.lg.ErrorD("ssh-request-subsytem", meta{"type": req.Type, "system": req.Payload[4:]})
								if string(req.Payload[4:]) == "sftp" {
									ok = true
								}
							}
						}
						m.lg.ErrorD("ssh-request-accepted", meta{"reply": ok, "type": req.Type})
						req.Reply(ok, nil)
					}
				}(requests)

				server, err := NewServer(channel, driver)
				if err != nil {
					m.lg.ErrorD("server-creation", meta{"err": err.Error()})
					return
				}
				if err := server.Serve(); err != nil {
					m.lg.ErrorD("server-closed", meta{"err": err.Error()})
					channel.Close()
				}
			}

		}(newConn)
	}
}
