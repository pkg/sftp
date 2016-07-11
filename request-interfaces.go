package sftp

import (
	"io"
	"os"
)

// Interfaces are differentiated based on required returned values.
// All input arguments are to be pulled from Request (the only arg).

// should return an io.Reader for the filepath
type FileReader interface {
	Fileread(*Request) (io.Reader, error)
}

// should return an io.Writer for the filepath
type FileWriter interface {
	Filewrite(*Request) (io.Writer, error)
}

// should return an error (rename, remove, setstate, etc.)
type FileCmder interface {
	Filecmd(*Request) error
}

// should return file listing info and errors (readdir, stat)
// note stat requests would return a list of 1
type FileInfoer interface {
	Fileinfo(*Request) ([]os.FileInfo, error)
}
