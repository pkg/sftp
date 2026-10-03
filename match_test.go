package sftp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobLiteralPaths(t *testing.T) {
	p := clientRequestServerPair(t)
	defer p.Close()

	require.NoError(t, p.cli.Mkdir("/home"))
	_, err := putTestFile(p.cli, "/home/file", "")
	require.NoError(t, err)

	for _, pattern := range []string{
		"/home/", "home/", "./home/", "/home//", "/home/./", "/home/../home/",
		"/", ".", "/home", "home/file", "./home/file", "/home/file",
	} {
		t.Run(pattern, func(t *testing.T) {
			matches, err := p.cli.Glob(pattern)
			require.NoError(t, err)
			assert.Equal(t, []string{pattern}, matches)
		})
	}

	matches, err := p.cli.Glob("/missing")
	require.NoError(t, err)
	assert.Nil(t, matches)
	matches, err = p.cli.Glob("/home/*")
	require.NoError(t, err)
	assert.Equal(t, []string{"/home/file"}, matches)
	_, err = p.cli.Glob("[")
	assert.ErrorIs(t, err, ErrBadPattern)
	checkRequestServerAllocator(t, p)
}
