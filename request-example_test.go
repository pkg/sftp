package sftp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A SETSTAT whose flags promise a size the packet does not carry leaves Attributes nil. The
// example handler must answer it, not dereference the nil and take the process down with it.
func TestInMemHandlerSetstatShortAttributes(t *testing.T) {
	fs := InMemHandler().FileCmd.(*root)

	_, err := fs.Filewrite(&Request{Method: "Put", Filepath: "/foo", Flags: sshFxfWrite | sshFxfCreat})
	require.NoError(t, err)

	err = fs.Filecmd(&Request{
		Method:   "Setstat",
		Filepath: "/foo",
		Flags:    sshFileXferAttrSize,
		Attrs:    []byte{0, 0, 0, 0}, // a size needs eight bytes
	})
	require.Error(t, err)
}
