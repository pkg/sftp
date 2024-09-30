package main

import (
	"fmt"
	"os"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

func main() {
	buf := make([]byte, sshfx.DefaultMaxPacketLength)

	for {
		var pkt sshfx.RawPacket
		if err := pkt.ReadFrom(os.Stdin, buf, sshfx.DefaultMaxPacketLength); err != nil {
			fmt.Fprintln(os.Stderr, "dump-packets:", err)
			os.Exit(1)
		}

		body, err := pkt.PacketBody()
		if err != nil {
			fmt.Printf("%s: %d: %#v\n", pkt.PacketType, pkt.RequestID, err)
		} else {
			fmt.Printf("%s: %d: %#v\n", pkt.PacketType, pkt.RequestID, body)
		}
	}
}
