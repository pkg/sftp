package localfs

import (
	sftp "github.com/pkg/sftp/v2"
)

var handler = &ServerHandler{}

// var _ sftp.HardlinkServerHandler = handler
var _ sftp.POSIXRenameServerHandler = handler
