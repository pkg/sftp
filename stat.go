package sftp

import (
	"os"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

// isRegular returns true if the mode describes a regular file.
func isRegular(mode uint32) bool {
	return sshfx.FileMode(mode)&sshfx.ModeType == sshfx.ModeRegular
}

// toFileMode converts sftp filemode bits to the os.FileMode specification
func toFileMode(mode uint32) os.FileMode {
	var fm = os.FileMode(mode & 0777)

	switch sshfx.FileMode(mode) & sshfx.ModeType {
	case sshfx.ModeDevice:
		fm |= os.ModeDevice
	case sshfx.ModeCharDevice:
		fm |= os.ModeDevice | os.ModeCharDevice
	case sshfx.ModeDir:
		fm |= os.ModeDir
	case sshfx.ModeNamedPipe:
		fm |= os.ModeNamedPipe
	case sshfx.ModeSymlink:
		fm |= os.ModeSymlink
	case sshfx.ModeRegular:
		// nothing to do
	case sshfx.ModeSocket:
		fm |= os.ModeSocket
	}

	if sshfx.FileMode(mode)&sshfx.ModeSetUID != 0 {
		fm |= os.ModeSetuid
	}
	if sshfx.FileMode(mode)&sshfx.ModeSetGID != 0 {
		fm |= os.ModeSetgid
	}
	if sshfx.FileMode(mode)&sshfx.ModeSticky != 0 {
		fm |= os.ModeSticky
	}

	return fm
}

// fromFileMode converts from the os.FileMode specification to sftp filemode bits
func fromFileMode(mode os.FileMode) uint32 {
	ret := sshfx.FileMode(mode & os.ModePerm)

	switch mode & os.ModeType {
	case os.ModeDevice | os.ModeCharDevice:
		ret |= sshfx.ModeCharDevice
	case os.ModeDevice:
		ret |= sshfx.ModeDevice
	case os.ModeDir:
		ret |= sshfx.ModeDir
	case os.ModeNamedPipe:
		ret |= sshfx.ModeNamedPipe
	case os.ModeSymlink:
		ret |= sshfx.ModeSymlink
	case 0:
		ret |= sshfx.ModeRegular
	case os.ModeSocket:
		ret |= sshfx.ModeSocket
	}

	if mode&os.ModeSetuid != 0 {
		ret |= sshfx.ModeSetUID
	}
	if mode&os.ModeSetgid != 0 {
		ret |= sshfx.ModeSetGID
	}
	if mode&os.ModeSticky != 0 {
		ret |= sshfx.ModeSticky
	}

	return uint32(ret)
}

const (
	s_ISUID = uint32(sshfx.ModeSetUID)
	s_ISGID = uint32(sshfx.ModeSetGID)
	s_ISVTX = uint32(sshfx.ModeSticky)
)

// S_IFMT is a legacy export. `sshfx.ModeType` should be used instead
const S_IFMT = uint32(sshfx.ModeType)
