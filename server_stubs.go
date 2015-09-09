// +build !cgo,!plan9 android

package sftp

func runLs(dirname string, dirent os.FileInfo) string {
	return path.Join(dirname, dirent.Name())
}
