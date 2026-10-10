//go:build !pkg_sftp.omit_ssh

package sftp

import "golang.org/x/crypto/ssh"

// NewClient creates a new SFTP client on conn, using zero or more option
// functions.
func NewClient(conn *ssh.Client, opts ...ClientOption) (*Client, error) {
	s, err := conn.NewSession()
	if err != nil {
		return nil, err
	}

	pw, err := s.StdinPipe()
	if err != nil {
		return nil, err
	}
	pr, err := s.StdoutPipe()
	if err != nil {
		return nil, err
	}
	perr, err := s.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := s.RequestSubsystem("sftp"); err != nil {
		return nil, err
	}

	return newClientPipe(pr, perr, pw, s.Wait, opts...)
}
