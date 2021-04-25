// +build plan9 windows android

package sftp

import (
	"os"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

func attributesFromFileInfo(fi os.FileInfo) sshfx.Attributes {
	return attributesFromGenericFileInfo(fi)
}
