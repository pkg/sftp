package sftp

import (
	"testing"
)

func TestChroot(t *testing.T) {
	s := &Server{rootDir: "/chroot"}
	for _, test := range [][]string{
		{"a", "/chroot/a"},
		{"a/b/..", "/chroot/a"},
		{"..", "/chroot"},
		{"a/../../b", "/chroot/b"},
		{"a/../../b/../..", "/chroot"},
	} {
		relPath, realPath := test[0], test[1]
		if s.chroot(relPath) != realPath {
			t.Fail()
		}
	}
}
