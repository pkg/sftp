package sftp_test

import (
	"log"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// reconnectingClient hands out a connected *sftp.Client, and connects again
// whenever the previous connection has gone away.
// It only connects when a client is asked for,
// so nothing reconnects while nobody needs the connection.
type reconnectingClient struct {
	addr   string
	config *ssh.ClientConfig

	mu     sync.Mutex
	client *sftp.Client
	done   chan struct{} // closed once client has disconnected
}

// Client returns a connected *sftp.Client, connecting first if necessary.
// Cached sessions are checked before reuse.
// Do not hold on to the returned client; ask for it again each time it is needed.
func (rc *reconnectingClient) Client() (*sftp.Client, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.client != nil {
		select {
		case <-rc.done:
			// disconnected, so connect again below
		default:
			// Wait's notification may still be pending after the session ends.
			if _, err := rc.client.RealPath("."); err == nil {
				return rc.client, nil
			}
			rc.client.Close()
		}
		rc.client = nil
	}

	conn, err := ssh.Dial("tcp", rc.addr, rc.config)
	if err != nil {
		return nil, err
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		// Wait returns once the SFTP session has ended, for whatever reason.
		// Closing the sftp.Client does not close the ssh.Client, so do that here.
		err := client.Wait()
		conn.Close()
		close(done)
		log.Print("sftp: disconnected: ", err)
	}()

	rc.client, rc.done = client, done
	return client, nil
}

// Close closes the current connection, if there is one.
func (rc *reconnectingClient) Close() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.client == nil {
		return nil
	}

	err := rc.client.Close()
	<-rc.done
	rc.client = nil

	return err
}

func Example_reconnect() {
	var config *ssh.ClientConfig

	rc := &reconnectingClient{
		addr:   "example.com:22",
		config: config,
	}
	defer rc.Close()

	for range time.Tick(time.Minute) {
		client, err := rc.Client()
		if err != nil {
			log.Print(err)
			continue
		}

		// If the connection drops while a request is in flight, the request fails
		// with sftp.ErrSSHFxConnectionLost, and the next call to rc.Client() connects again.
		fi, err := client.Stat("/upload/ready")
		if err != nil {
			log.Print(err)
			continue
		}
		log.Print(fi.Name(), " is ready")
	}
}
