package main

// password implements the ClientPassword interface
type password string

func (p password) Password(user string) (string, error) {
	return string(p), nil
}
