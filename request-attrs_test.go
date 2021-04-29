package sftp

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

func TestRequestPflags(t *testing.T) {
	r := Request{
		Flags: sshfx.FlagRead | sshfx.FlagWrite | sshfx.FlagAppend,
	}

	pflags := r.Pflags()

	assert.True(t, pflags.Read)
	assert.True(t, pflags.Write)
	assert.True(t, pflags.Append)
	assert.False(t, pflags.Creat)
	assert.False(t, pflags.Trunc)
	assert.False(t, pflags.Excl)
}

func TestRequestAflags(t *testing.T) {
	r := Request{
		Flags: sshfx.AttrSize | sshfx.AttrUIDGID,
	}

	aflags := r.AttrFlags()

	assert.True(t, aflags.Size)
	assert.True(t, aflags.UidGid)
	assert.False(t, aflags.Acmodtime)
	assert.False(t, aflags.Permissions)
}

func TestRequestAttributes(t *testing.T) {
	// UID/GID
	r := Request{
		Flags: sshfx.AttrUIDGID,
		Attrs: []byte{
			0x00, 0x00, 0x00, 1,
			0x00, 0x00, 0x00, 2,
		},
	}

	want := FileStat{UID: 1, GID: 2}
	testFs := r.Attributes()
	assert.Equal(t, want, *testFs)

	// Size and Mode
	r = Request{
		Flags: sshfx.AttrSize | sshfx.AttrPermissions,
		Attrs: []byte{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 99,
			0x00, 0x00, 0x01, 0xe9, // 0751
		},
	}

	want = FileStat{Mode: 0751, Size: 99}
	testFs = r.Attributes()
	assert.Equal(t, want, *testFs)

	// FileMode
	assert.True(t, testFs.FileMode().IsRegular())
	assert.False(t, testFs.FileMode().IsDir())
	assert.Equal(t, testFs.FileMode().Perm(), os.FileMode(0751).Perm())
}

func TestRequestAttributesEmpty(t *testing.T) {
	const (
		sshfxAttrAll = sshfx.AttrSize | sshfx.AttrUIDGID | sshfx.AttrPermissions |
			sshfx.AttrACModTime | sshfx.AttrExtended
	)

	// Size and Mode
	r := Request{
		Flags: sshfxAttrAll,
		Attrs: []byte{},
	}

	testFs := r.Attributes()
	assert.Equal(t, FileStat{}, *testFs)
}
