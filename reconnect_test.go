package sftp_test

import (
	"net"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func newReconnectTestClient(t *testing.T) *sftp.Client {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	server, err := sftp.NewServer(serverConn)
	if err != nil {
		clientConn.Close()
		serverConn.Close()
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.Serve()
	}()
	t.Cleanup(func() {
		clientConn.Close()
		server.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("SFTP server did not stop")
		}
	})
	client, err := sftp.NewClientPipe(clientConn, clientConn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestReconnectingClientReusesLiveSession(t *testing.T) {
	client := newReconnectTestClient(t)
	rc := &reconnectingClient{client: client, done: make(chan struct{})}
	got, err := rc.Client()
	if err != nil || got != client {
		t.Fatalf("Client() = %v, %v; want the live cached client", got, err)
	}
}

func TestReconnectingClientDiscardsDisconnectedSession(t *testing.T) {
	for _, name := range []string{"notification pending", "notification delivered"} {
		t.Run(name, func(t *testing.T) {
			client := newReconnectTestClient(t)
			client.Close()
			client.Wait()
			done := make(chan struct{})
			if name == "notification delivered" {
				close(done)
			}
			rc := &reconnectingClient{
				addr:   "missing-port",
				config: &ssh.ClientConfig{},
				client: client,
				done:   done,
			}
			got, err := rc.Client()
			if err == nil || got != nil {
				t.Fatalf("Client() = %v, %v; want the reconnect attempt's dial error", got, err)
			}
			if rc.client != nil {
				t.Fatal("disconnected session remains cached after reconnect failed")
			}
		})
	}
}
