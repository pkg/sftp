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

// Alerter is the function signature for an optional alerting function to be called in error cases.
type Alerter func(title string, metadata map[string]interface{})

// DriverGenerator is a function that creates an SFTP ServerDriver if the login request
// is valid.
type DriverGenerator func(LoginRequest) ServerDriver

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
	alertFn         Alerter
}

// NewManagedServer creates a new ManagedServer which conditionally serves requests based
// on the output of driverGenerator.
func NewManagedServer(driverGenerator DriverGenerator, lg Logger, alertFn Alerter) *ManagedServer {
	return &ManagedServer{
		driverGenerator: driverGenerator,
		lg:              lg,
		alertFn:         alertFn,
	}
}

func (m ManagedServer) errorAndAlert(title string, metadata map[string]interface{}) {
	if m.alertFn != nil {
		m.alertFn(title, metadata)
	}
	m.lg.ErrorD(title, metadata)
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
			m.errorAndAlert("private-key-parse", meta{"index": i, "error": err.Error()})
			os.Exit(1)
		}
		privateKeys = append(privateKeys, privateKey)
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", port))
	proxyList := Listener{Listener: listener}

	if err != nil {
		m.errorAndAlert("listen-fail", meta{
			"msg":   "failed to open socket",
			"error": err.Error(),
			"port":  port})
	}
	m.lg.InfoD("listening", meta{"address": proxyList.Addr().String()})

	for {
		newConn, err := proxyList.Accept()
		if err != nil {
			m.errorAndAlert("listener-accept-fail", meta{"error": err.Error()})
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
					driver = m.driverGenerator(LoginRequest{
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
					m.errorAndAlert("handshake-failure", meta{"error": err.Error()})
				}
				return
			}

			go ssh.DiscardRequests(requestChan)

			for newChannelRequest := range newChan {
				if newChannelRequest.ChannelType() != "session" {
					newChannelRequest.Reject(ssh.UnknownChannelType, "unknown channel type")
					m.errorAndAlert("unknown-channel-type", meta{"type": newChannelRequest.ChannelType()})
					continue
				}
				channel, requests, err := newChannelRequest.Accept()
				if err != nil {
					if err != io.EOF {
						m.errorAndAlert("channel-accept-failure", meta{
							"err":  err.Error(),
							"type": newChannelRequest.ChannelType()})
					}
					return
				}

				go func(in <-chan *ssh.Request) {
					for req := range in {
						ok := false
						switch req.Type {
						case "subsystem":
							if len(req.Payload) >= 4 {
								// we reject all SSH requests that are not SFTP
								if string(req.Payload[4:]) == "sftp" {
									ok = true
								}
							}
						}
						req.Reply(ok, nil)
					}
				}(requests)

				server, err := NewServer(channel, driver)
				if err != nil {
					m.errorAndAlert("server-creation-err", meta{"err": err.Error()})
					return
				}
				if err := server.Serve(); err != nil {
					channel.Close()
				}
			}
		}(newConn)
	}
}
