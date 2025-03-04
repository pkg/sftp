package localfs

import (
	"path"
	"path/filepath"
)

func toLocalPath(p string) (string, error) {
	if path.IsAbs(p) {
		tmp := p[1:]

		if filepath.IsAbs(tmp) {
			// If the FromSlash without any starting slashes is absolute,
			// then we have a filepath encoded with a prefix '/'.
			// e.g. "/#s/boot" to "#s/boot"
			return tmp, nil
		}
	}

	return p, nil
}
