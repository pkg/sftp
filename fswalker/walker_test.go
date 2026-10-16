package fswalker_test

import (
	"fmt"
	"os"

	"github.com/pkg/sftp/fswalker"
)

func ExampleWalker() {
	walker := fswalker.Walk("/usr/lib")
	for walker.Step() {
		if err := walker.Err(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		fmt.Println(walker.Path())
	}
}
