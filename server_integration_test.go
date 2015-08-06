package sftp

// sftp server integration tests
// enable with -integration

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/ScriptRock/crypto/ssh"
)

var testSftpClientBin = flag.String("sftp_client", "/usr/bin/sftp", "location of the sftp client binary")
var sshServerDebugStream = os.Stdout  // ioutil.Discard
var sftpServerDebugStream = os.Stdout // ioutil.Discard
var sftpClientDebugStream = os.Stdout // ioutil.Discard

/***********************************************************************************************


SSH server scaffolding; very simple, no strict auth. This is for unit testing, not real servers


***********************************************************************************************/

var (
	hostPrivateKeySigner ssh.Signer
	privKey              = []byte(`
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEArhp7SqFnXVZAgWREL9Ogs+miy4IU/m0vmdkoK6M97G9NX/Pj
wf8I/3/ynxmcArbt8Rc4JgkjT2uxx/NqR0yN42N1PjO5Czu0dms1PSqcKIJdeUBV
7gdrKSm9Co4d2vwfQp5mg47eG4w63pz7Drk9+VIyi9YiYH4bve7WnGDswn4ycvYZ
slV5kKnjlfCdPig+g5P7yQYud0cDWVwyA0+kxvL6H3Ip+Fu8rLDZn4/P1WlFAIuc
PAf4uEKDGGmC2URowi5eesYR7f6GN/HnBs2776laNlAVXZUmYTUfOGagwLsEkx8x
XdNqntfbs2MOOoK+myJrNtcB9pCrM0H6um19uQIDAQABAoIBABkWr9WdVKvalgkP
TdQmhu3mKRNyd1wCl+1voZ5IM9Ayac/98UAvZDiNU4Uhx52MhtVLJ0gz4Oa8+i16
IkKMAZZW6ro/8dZwkBzQbieWUFJ2Fso2PyvB3etcnGU8/Yhk9IxBDzy+BbuqhYE2
1ebVQtz+v1HvVZzaD11bYYm/Xd7Y28QREVfFen30Q/v3dv7dOteDE/RgDS8Czz7w
jMW32Q8JL5grz7zPkMK39BLXsTcSYcaasT2ParROhGJZDmbgd3l33zKCVc1zcj9B
SA47QljGd09Tys958WWHgtj2o7bp9v1Ufs4LnyKgzrB80WX1ovaSQKvd5THTLchO
kLIhUAECgYEA2doGXy9wMBmTn/hjiVvggR1aKiBwUpnB87Hn5xCMgoECVhFZlT6l
WmZe7R2klbtG1aYlw+y+uzHhoVDAJW9AUSV8qoDUwbRXvBVlp+In5wIqJ+VjfivK
zgIfzomL5NvDz37cvPmzqIeySTowEfbQyq7CUQSoDtE9H97E2wWZhDkCgYEAzJdJ
k+NSFoTkHhfD3L0xCDHpRV3gvaOeew8524fVtVUq53X8m91ng4AX1r74dCUYwwiF
gqTtSSJfx2iH1xKnNq28M9uKg7wOrCKrRqNPnYUO3LehZEC7rwUr26z4iJDHjjoB
uBcS7nw0LJ+0Zeg1IF+aIdZGV3MrAKnrzWPixYECgYBsffX6ZWebrMEmQ89eUtFF
u9ZxcGI/4K8ErC7vlgBD5ffB4TYZ627xzFWuBLs4jmHCeNIJ9tct5rOVYN+wRO1k
/CRPzYUnSqb+1jEgILL6istvvv+DkE+ZtNkeRMXUndWwel94BWsBnUKe0UmrSJ3G
sq23J3iCmJW2T3z+DpXbkQKBgQCK+LUVDNPE0i42NsRnm+fDfkvLP7Kafpr3Umdl
tMY474o+QYn+wg0/aPJIf9463rwMNyyhirBX/k57IIktUdFdtfPicd2MEGETElWv
nN1GzYxD50Rs2f/jKisZhEwqT9YNyV9DkgDdGGdEbJNYqbv0qpwDIg8T9foe8E1p
bdErgQKBgAt290I3L316cdxIQTkJh1DlScN/unFffITwu127WMr28Jt3mq3cZpuM
Aecey/eEKCj+Rlas5NDYKsB18QIuAw+qqWyq0LAKLiAvP1965Rkc4PLScl3MgJtO
QYa37FK0p8NcDeUuF86zXBVutwS5nJLchHhKfd590ks57OROtm29
-----END RSA PRIVATE KEY-----
`)
)

func init() {
	var err error
	hostPrivateKeySigner, err = ssh.ParsePrivateKey(privKey)
	if err != nil {
		panic(err)
	}
}

func keyAuth(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	permissions := &ssh.Permissions{
		CriticalOptions: map[string]string{},
		Extensions:      map[string]string{},
	}
	return permissions, nil
}

func pwAuth(conn ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
	permissions := &ssh.Permissions{
		CriticalOptions: map[string]string{},
		Extensions:      map[string]string{},
	}
	return permissions, nil
}

func basicServerConfig() *ssh.ServerConfig {
	config := ssh.ServerConfig{
		Config: ssh.Config{
			MACs: []string{"hmac-sha1"},
		},
		PasswordCallback:  pwAuth,
		PublicKeyCallback: keyAuth,
	}
	config.AddHostKey(hostPrivateKeySigner)
	return &config
}

type sshServer struct {
	conn     net.Conn
	config   *ssh.ServerConfig
	sshConn  *ssh.ServerConn
	newChans <-chan ssh.NewChannel
	newReqs  <-chan *ssh.Request
}

func sshServerFromConn(conn net.Conn, config *ssh.ServerConfig) (*sshServer, error) {
	// From a standard TCP connection to an encrypted SSH connection
	sshConn, newChans, newReqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return nil, err
	}

	svr := &sshServer{conn, config, sshConn, newChans, newReqs}
	svr.listenChannels()
	return svr, nil
}

func (svr *sshServer) Wait() error {
	return svr.sshConn.Wait()
}

func (svr *sshServer) Close() error {
	return svr.sshConn.Close()
}

func (svr *sshServer) listenChannels() {
	go func() {
		for chanReq := range svr.newChans {
			go svr.handleChanReq(chanReq)
		}
	}()
	go func() {
		for req := range svr.newReqs {
			go svr.handleReq(req)
		}
	}()
}

func (svr *sshServer) handleReq(req *ssh.Request) {
	switch req.Type {
	default:
		rejectRequest(req)
	}
}

type sshChannelServer struct {
	svr     *sshServer
	chanReq ssh.NewChannel
	ch      ssh.Channel
	newReqs <-chan *ssh.Request
}

type sshSessionChannelServer struct {
	*sshChannelServer
	env []string
}

func (svr *sshServer) handleChanReq(chanReq ssh.NewChannel) {
	fmt.Fprintf(sshServerDebugStream, "channel request: %v, extra: '%v'\n", chanReq.ChannelType(), hex.EncodeToString(chanReq.ExtraData()))
	switch chanReq.ChannelType() {
	case "session":
		if ch, reqs, err := chanReq.Accept(); err != nil {
			fmt.Fprintf(sshServerDebugStream, "fail to accept channel request: %v\n", err)
			chanReq.Reject(ssh.ResourceShortage, "channel accept failure")
		} else {
			chsvr := &sshSessionChannelServer{
				sshChannelServer: &sshChannelServer{svr, chanReq, ch, reqs},
				env:              append([]string{}, os.Environ()...),
			}
			chsvr.handle()
		}
	default:
		chanReq.Reject(ssh.UnknownChannelType, "channel type is not a session")
	}
}

func (chsvr *sshSessionChannelServer) handle() {
	// should maybe do something here...
	go chsvr.handleReqs()
}

func (chsvr *sshSessionChannelServer) handleReqs() {
	for req := range chsvr.newReqs {
		chsvr.handleReq(req)
	}
	fmt.Fprintf(sshServerDebugStream, "ssh server session channel complete\n")
}

func (chsvr *sshSessionChannelServer) handleReq(req *ssh.Request) {
	switch req.Type {
	case "env":
		chsvr.handleEnv(req)
	case "subsystem":
		chsvr.handleSubsystem(req)
	default:
		rejectRequest(req)
	}
}

func rejectRequest(req *ssh.Request) error {
	fmt.Fprintf(sshServerDebugStream, "ssh rejecting request, type: %s\n", req.Type)
	err := req.Reply(false, []byte{})
	if err != nil {
		fmt.Fprintf(sshServerDebugStream, "ssh request reply had error: %v\n", err)
	}
	return err
}

func rejectRequestUnmarshalError(req *ssh.Request, s interface{}, err error) error {
	fmt.Fprintf(sshServerDebugStream, "ssh request unmarshaling error, type '%T': %v\n", s, err)
	rejectRequest(req)
	return err
}

// env request form:
type sshEnvRequest struct {
	Envvar string
	Value  string
}

func (chsvr *sshSessionChannelServer) handleEnv(req *ssh.Request) error {
	envReq := &sshEnvRequest{}
	if err := ssh.Unmarshal(req.Payload, envReq); err != nil {
		return rejectRequestUnmarshalError(req, envReq, err)
	}
	req.Reply(true, nil)

	found := false
	for i, envstr := range chsvr.env {
		if strings.HasPrefix(envstr, envReq.Envvar+"=") {
			found = true
			chsvr.env[i] = envReq.Envvar + "=" + envReq.Value
		}
	}
	if !found {
		chsvr.env = append(chsvr.env, envReq.Envvar+"="+envReq.Value)
	}

	return nil
}

// Payload: int: command size, string: command
type sshSubsystemRequest struct {
	Name string
}

type sshSubsystemExitStatus struct {
	Status uint32
}

func (chsvr *sshSessionChannelServer) handleSubsystem(req *ssh.Request) error {
	defer func() {
		err1 := chsvr.ch.CloseWrite()
		err2 := chsvr.ch.Close()
		fmt.Fprintf(sshServerDebugStream, "ssh server subsystem request complete, err: %v %v\n", err1, err2)
	}()

	subsystemReq := &sshSubsystemRequest{}
	if err := ssh.Unmarshal(req.Payload, subsystemReq); err != nil {
		return rejectRequestUnmarshalError(req, subsystemReq, err)
	}

	// reply to the ssh client

	// no idea if this is actually correct spec-wise.
	// just enough for an sftp server to start.
	if subsystemReq.Name == "sftp" {
		req.Reply(true, nil)

		if false {
			// use the sftp server backend; this is to test the ssh code, not the sftp code
			cmd := exec.Command(*testSftp, "-e", "-l", "DEBUG") // log to stderr
			cmd.Stdin = chsvr.ch
			cmd.Stdout = chsvr.ch
			cmd.Stderr = sftpServerDebugStream
			if err := cmd.Start(); err != nil {
				return err
			}
			return cmd.Wait()
		} else {
			sftpServer, err := NewServer(chsvr.ch, chsvr.ch, sftpServerDebugStream, 0, false, ".")
			if err != nil {
				return err
			}

			// wait for the session to close
			runErr := sftpServer.Run()
			exitStatus := uint32(1)
			if runErr == nil {
				exitStatus = uint32(0)
			}

			_, exitStatusErr := chsvr.ch.SendRequest("exit-status", false, ssh.Marshal(sshSubsystemExitStatus{exitStatus}))
			return exitStatusErr
		}
	} else {
		return req.Reply(false, nil)
	}
}

/***********************************************************************************************


Actual unit tests


***********************************************************************************************/

// starts an ssh server to test. returns: host string and port
func testServer(t *testing.T, readonly bool) (string, int) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	host, portStr, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Fprintf(sshServerDebugStream, "ssh server socket closed\n")
				break
			}

			go func() {
				defer conn.Close()
				sshSvr, err := sshServerFromConn(conn, basicServerConfig())
				if err != nil {
					t.Fatal(err)
				}
				err = sshSvr.Wait()
				fmt.Fprintf(sshServerDebugStream, "ssh server finished, err: %v\n", err)
			}()
		}
	}()

	return host, port
}

func runSftpClient(script string, path string, host string, port int) (string, error) {
	cmd := exec.Command(*testSftpClientBin, "-vvvv", "-b", "-", "-o", "StrictHostKeyChecking=no", "-o", "LogLevel=ERROR", "-o", "UserKnownHostsFile /dev/null", "-P", fmt.Sprintf("%d", port), fmt.Sprintf("%s:%s", host, path))
	stdout := &bytes.Buffer{}
	cmd.Stdin = bytes.NewBufferString(script)
	cmd.Stdout = stdout
	cmd.Stderr = sftpClientDebugStream
	if err := cmd.Start(); err != nil {
		return "", err
	}
	err := cmd.Wait()
	return string(stdout.Bytes()), err
}

func TestServerLstat(t *testing.T) {
	host, port := testServer(t, READONLY)

	script := "ls"
	output, err := runSftpClient(script, "/tmp/", host, port)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(output)
}
