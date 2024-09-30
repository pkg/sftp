package sftp

import (
	"context"
	"fmt"

	sshfx "github.com/pkg/sftp/v2/encoding/ssh/filexfer"
)

type UnimplementedHandler struct{}

func (UnimplementedHandler) Mkdir(_ context.Context, req *sshfx.MkdirPacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

func (UnimplementedHandler) Remove(_ context.Context, req *sshfx.RemovePacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

func (UnimplementedHandler) Rename(_ context.Context, req *sshfx.RenamePacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

func (UnimplementedHandler) Rmdir(_ context.Context, req *sshfx.RmdirPacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
	return sshfx.StatusOPUnsupported
}

func (UnimplementedHandler) SetStat(_ context.Context, req *sshfx.SetStatPacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
	return sshfx.StatusOPUnsupported
}

func (UnimplementedHandler) Symlink(_ context.Context, req *sshfx.SymlinkPacket) error {
	return &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
	return sshfx.StatusOPUnsupported
}

func (UnimplementedHandler) LStat(_ context.Context, req *sshfx.LStatPacket) (*sshfx.Attributes, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

func (UnimplementedHandler) Stat(_ context.Context, req *sshfx.StatPacket) (*sshfx.Attributes, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

func (UnimplementedHandler) ReadLink(_ context.Context, req *sshfx.ReadLinkPacket) (string, error) {
	return "", &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

func (UnimplementedHandler) RealPath(_ context.Context, req *sshfx.RealPathPacket) (string, error) {
	return "", &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

func (UnimplementedHandler) Open(_ context.Context, req *sshfx.OpenPacket) (FileHandler, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

func (UnimplementedHandler) OpenDir(_ context.Context, req *sshfx.OpenDirPacket) (DirHandler, error) {
	return nil, &sshfx.StatusPacket{
		StatusCode:   sshfx.StatusOPUnsupported,
		ErrorMessage: fmt.Sprint(req.Type()),
	}
}

var directImpl = map[string]bool{
	"Mkdir":    true,
	"Remove":   true,
	"Rename":   true,
	"Rmdir":    true,
	"SetStat":  true,
	"Symlink":  true,
	"LStat":    true,
	"Stat":     true,
	"ReadLink": true,
	"RealPath": true,
	"Open":     true,
	"OpenDir":  true,
}
