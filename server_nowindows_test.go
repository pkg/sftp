//go:build !windows
// +build !windows

package sftp

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerSymlinkTargets(t *testing.T) {
	for _, withWorkDir := range []bool{false, true} {
		name := "without working directory"
		if withWorkDir {
			name = "with working directory"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "file"), []byte("subdirectory"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), []byte("parent"), 0o600))

			var options []ServerOption
			if withWorkDir {
				options = append(options, WithServerWorkingDirectory(dir))
			}
			client, server := clientServerPair(t, options...)
			defer client.Close()
			defer server.Close()

			for _, tt := range []struct {
				name, target, contents string
			}{
				{"relative", "file", "subdirectory"},
				{"parent", "../file", "parent"},
				{"absolute", filepath.ToSlash(filepath.Join(dir, "file")), "parent"},
				{"dangling", "missing", ""},
			} {
				t.Run(tt.name, func(t *testing.T) {
					link := "sub/" + tt.name
					if !withWorkDir {
						link = filepath.ToSlash(filepath.Join(dir, link))
					}
					require.NoError(t, client.Symlink(tt.target, link))
					target, err := client.ReadLink(link)
					require.NoError(t, err)
					require.Equal(t, tt.target, target)
					if tt.name == "dangling" {
						_, err := client.Stat(link)
						require.True(t, os.IsNotExist(err), "stat dangling link: %v", err)
						return
					}
					file, err := client.Open(link)
					require.NoError(t, err)
					defer file.Close()
					contents, err := io.ReadAll(file)
					require.NoError(t, err)
					require.Equal(t, tt.contents, string(contents))
				})
			}
		})
	}
}

func TestServer_toLocalPath(t *testing.T) {
	tests := []struct {
		name        string
		withWorkDir string
		p           string
		want        string
	}{
		{
			name: "empty path with no workdir",
			p:    "",
			want: "",
		},
		{
			name: "relative path with no workdir",
			p:    "file",
			want: "file",
		},
		{
			name: "absolute path with no workdir",
			p:    "/file",
			want: "/file",
		},
		{
			name:        "workdir and empty path",
			withWorkDir: "/home/user",
			p:           "",
			want:        "/home/user",
		},
		{
			name:        "workdir and relative path",
			withWorkDir: "/home/user",
			p:           "file",
			want:        "/home/user/file",
		},
		{
			name:        "workdir and relative path with .",
			withWorkDir: "/home/user",
			p:           ".",
			want:        "/home/user",
		},
		{
			name:        "workdir and relative path with . and file",
			withWorkDir: "/home/user",
			p:           "./file",
			want:        "/home/user/file",
		},
		{
			name:        "workdir and absolute path",
			withWorkDir: "/home/user",
			p:           "/file",
			want:        "/file",
		},
		{
			name:        "workdir and non-unixy path prefixes workdir",
			withWorkDir: "/home/user",
			p:           "C:\\file",
			// This may look like a bug but it is the result of passing
			// invalid input (a non-unixy path) to the server.
			want: "/home/user/C:\\file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We don't need to initialize the Server further to test
			// toLocalPath behavior.
			s := &Server{}
			if tt.withWorkDir != "" {
				if err := WithServerWorkingDirectory(tt.withWorkDir)(s); err != nil {
					t.Fatal(err)
				}
			}

			if got := s.toLocalPath(tt.p); got != tt.want {
				t.Errorf("Server.toLocalPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
