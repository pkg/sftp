//go:build !plan9 && !windows && !zos && (!js || !wasm)
// +build !plan9
// +build !windows
// +build !zos
// +build !js !wasm

package sftp

import "syscall"

const S_IFMT = syscall.S_IFMT
