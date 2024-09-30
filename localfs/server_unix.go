//go:build !windows && !plan9
// +build !windows,!plan9

package localfs

func toLocalPath(p string) (string, error) {
	return p, nil
}
