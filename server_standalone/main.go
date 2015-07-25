package main

// small wrapper around sftp server that allows it to be used as a separate process subsystem call by the ssh server.
// in practice this will statically link; however this allows unit testing from the sftp client.

import (
	"os"

	"github.com/ScriptRock/sftp"
)

func main() {
	svr, _ := sftp.NewServer(os.Stdin, os.Stdout, "")
	svr.Run()
}
