module github.com/pkg/sftp/v2/examples

go 1.25

require (
	github.com/pkg/sftp/v2 v2.0.0-alpha
	golang.org/x/crypto v0.36.0
)

require golang.org/x/sys v0.31.0 // indirect

replace github.com/pkg/sftp/v2 => ..
