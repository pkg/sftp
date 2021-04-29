package sftp

// Methods on the Request object to make working with the Flags bitmasks and
// Attr(ibutes) byte blob easier. Use Pflags() when working with an Open/Write
// request and AttrFlags() and Attributes() when working with SetStat requests.
import (
	"os"

	sshfx "github.com/pkg/sftp/internal/encoding/ssh/filexfer"
)

// FileOpenFlags defines Open and Write Flags. Correlate directly with with os.OpenFile flags
// (https://golang.org/pkg/os/#pkg-constants).
type FileOpenFlags struct {
	Read, Write, Append, Creat, Trunc, Excl bool
}

// Pflags converts the bitmap/uint32 from SFTP Open packet pflag values,
// into a FileOpenFlags struct with booleans set for flags set in bitmap.
func (r *Request) Pflags() FileOpenFlags {
	return FileOpenFlags{
		Read:   r.Flags&sshfx.FlagRead != 0,
		Write:  r.Flags&sshfx.FlagWrite != 0,
		Append: r.Flags&sshfx.FlagAppend != 0,
		Creat:  r.Flags&sshfx.FlagCreate != 0,
		Trunc:  r.Flags&sshfx.FlagTruncate != 0,
		Excl:   r.Flags&sshfx.FlagExclusive != 0,
	}
}

// FileAttrFlags that indicate whether SFTP file attributes were passed. When a flag is
// true the corresponding attribute should be available from the FileStat
// object returned by Attributes method. Used with SetStat.
type FileAttrFlags struct {
	Size, UidGid, Permissions, Acmodtime bool
}

// AttrFlags returns a FileAttrFlags boolean struct based on the
// bitmap/uint32 file attribute flags from the SFTP packaet.
func (r *Request) AttrFlags() FileAttrFlags {
	return FileAttrFlags{
		Size:        r.Flags&sshfx.AttrSize != 0,
		UidGid:      r.Flags&sshfx.AttrUIDGID != 0,
		Permissions: r.Flags&sshfx.AttrPermissions != 0,
		Acmodtime:   r.Flags&sshfx.AttrACModTime != 0,
	}
}

// FileMode returns the Mode SFTP file attributes wrapped as os.FileMode
func (a FileStat) FileMode() os.FileMode {
	return os.FileMode(a.Mode)
}

// Attributes parses file attributes byte blob and return them in a
// FileStat object.
func (r *Request) Attributes() *FileStat {
	var attrs sshfx.Attributes

	_ = attrs.XXX_UnmarshalByFlags(r.Flags, sshfx.NewBuffer(r.Attrs))

	fs := fromAttributes(attrs)

	return &fs
}
