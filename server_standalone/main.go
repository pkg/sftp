package main

// small wrapper around sftp server that allows it to be used as a separate process subsystem call by the ssh server.
// in practice this will statically link; however this allows unit testing from the sftp client.

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pkg/sftp"
)

func main() {
	var (
		readOnly      bool
		debugLevelStr string
		debugLevel    int
		debugStderr   bool
	)

	flag.BoolVar(&readOnly, "R", false, "read-only server")
	flag.BoolVar(&debugStderr, "e", false, "debug to stderr")
	flag.StringVar(&debugLevelStr, "l", "none", "debug level")
	flag.Parse()

	debugStream := ioutil.Discard
	if debugStderr {
		debugStream = os.Stderr
		debugLevel = 1
	}

	svr, _ := sftp.NewServer(os.Stdin, os.Stdout, debugStream, debugLevel, readOnly, "")
	if err := svr.Serve(); err != nil {
		fmt.Fprintf(debugStream, "sftp server completed with error: %v", err)
		os.Exit(1)
	}
}
