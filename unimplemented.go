package sftp

import (
	"context"
	"fmt"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

// UnimplementedServerHandler must be embedded to both ensure forward compatible implementations,
// but also stubs out any functions that you do not wish to implement.
type UnimplementedServerHandler struct{}

func (UnimplementedServerHandler) mustEmbedUnimplementedServerHandler() {}

// Mkdir returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) Mkdir(_ context.Context, req *sshfx.MkdirPacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

// Remove returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) Remove(_ context.Context, req *sshfx.RemovePacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

// Rename returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) Rename(_ context.Context, req *sshfx.RenamePacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

// Rmdir returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) Rmdir(_ context.Context, req *sshfx.RmdirPacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
	return sshfx.StatusOpUnsupported
}

// SetStat returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) SetStat(_ context.Context, req *sshfx.SetStatPacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
	return sshfx.StatusOpUnsupported
}

// Symlink returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) Symlink(_ context.Context, req *sshfx.SymlinkPacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
	return sshfx.StatusOpUnsupported
}

// LStat returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) LStat(_ context.Context, req *sshfx.LStatPacket) (*sshfx.Attributes, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

// Stat returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) Stat(_ context.Context, req *sshfx.StatPacket) (*sshfx.Attributes, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

// ReadLink returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) ReadLink(_ context.Context, req *sshfx.ReadLinkPacket) (string, error) {
	return "", &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

// RealPath returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) RealPath(_ context.Context, req *sshfx.RealPathPacket) (string, error) {
	return "", &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

// Open returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) Open(_ context.Context, req *sshfx.OpenPacket) (FileHandler, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

// OpenDir returns an sshfx.StatusOpUnsupported error.
func (UnimplementedServerHandler) OpenDir(_ context.Context, req *sshfx.OpenDirPacket) (DirHandler, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOpUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}
