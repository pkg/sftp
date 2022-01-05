package sftp

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clientServerPair(t *testing.T, options ...ServerOption) (*Client, *Server) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	if *testAllocator {
		options = append(options, WithAllocator())
	}
	server, err := NewServer(struct {
		io.Reader
		io.WriteCloser
	}{sr, sw}, options...)
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve()
	client, err := NewClientPipe(cr, cw)
	if err != nil {
		t.Fatalf("%+v\n", err)
	}
	return client, server
}

type sshFxpTestBadExtendedPacket struct {
	ID        uint32
	Extension string
	Data      string
}

func (p sshFxpTestBadExtendedPacket) id() uint32 { return p.ID }

func (p sshFxpTestBadExtendedPacket) MarshalBinary() ([]byte, error) {
	l := 4 + 1 + 4 + // uint32(length) + byte(type) + uint32(id)
		4 + len(p.Extension) +
		4 + len(p.Data)

	b := make([]byte, 4, l)
	b = append(b, sshFxpExtended)
	b = marshalUint32(b, p.ID)
	b = marshalString(b, p.Extension)
	b = marshalString(b, p.Data)

	return b, nil
}

func checkServerAllocator(t *testing.T, server *Server) {
	if server.pktMgr.alloc == nil {
		return
	}
	checkAllocatorBeforeServerClose(t, server.pktMgr.alloc)
	server.Close()
	checkAllocatorAfterServerClose(t, server.pktMgr.alloc)
}

// test that errors are sent back when we request an invalid extended packet operation
// this validates the following rfc draft is followed https://tools.ietf.org/html/draft-ietf-secsh-filexfer-extensions-00
func TestInvalidExtendedPacket(t *testing.T) {
	client, server := clientServerPair(t)
	defer client.Close()
	defer server.Close()

	badPacket := sshFxpTestBadExtendedPacket{client.nextID(), "thisDoesn'tExist", "foobar"}
	typ, data, err := client.clientConn.sendPacket(nil, badPacket)
	if err != nil {
		t.Fatalf("unexpected error from sendPacket: %s", err)
	}
	if typ != sshFxpStatus {
		t.Fatalf("received non-FPX_STATUS packet: %v", typ)
	}

	err = unmarshalStatus(badPacket.id(), data)
	statusErr, ok := err.(*StatusError)
	if !ok {
		t.Fatal("failed to convert error from unmarshalStatus to *StatusError")
	}
	if statusErr.Code != sshFxOPUnsupported {
		t.Errorf("statusErr.Code => %d, wanted %d", statusErr.Code, sshFxOPUnsupported)
	}
	checkServerAllocator(t, server)
}

// test that server handles concurrent requests correctly
func TestConcurrentRequests(t *testing.T) {
	skipIfWindows(t)
	filename := "/etc/passwd"
	if runtime.GOOS == "plan9" {
		filename = "/lib/ndb/local"
	}
	client, server := clientServerPair(t)
	defer client.Close()
	defer server.Close()

	concurrency := 2
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < 1024; j++ {
				f, err := client.Open(filename)
				if err != nil {
					t.Errorf("failed to open file: %v", err)
					continue
				}
				if err := f.Close(); err != nil {
					t.Errorf("failed t close file: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	checkServerAllocator(t, server)
}

// Test error conversion
func TestStatusFromError(t *testing.T) {
	type test struct {
		err error
		pkt *sshFxpStatusPacket
	}
	tpkt := func(id, code uint32) *sshFxpStatusPacket {
		return &sshFxpStatusPacket{
			ID:          id,
			StatusError: StatusError{Code: code},
		}
	}
	testCases := []test{
		{syscall.ENOENT, tpkt(1, sshFxNoSuchFile)},
		{&os.PathError{Err: syscall.ENOENT},
			tpkt(2, sshFxNoSuchFile)},
		{&os.PathError{Err: errors.New("foo")}, tpkt(3, sshFxFailure)},
		{ErrSSHFxEOF, tpkt(4, sshFxEOF)},
		{ErrSSHFxOpUnsupported, tpkt(5, sshFxOPUnsupported)},
		{io.EOF, tpkt(6, sshFxEOF)},
		{os.ErrNotExist, tpkt(7, sshFxNoSuchFile)},
	}
	for _, tc := range testCases {
		tc.pkt.StatusError.msg = tc.err.Error()
		assert.Equal(t, tc.pkt, statusFromError(tc.pkt.ID, tc.err))
	}
}

// This was written to test a race b/w open immediately followed by a stat.
// Previous to this the Open would trigger the use of a worker pool, then the
// stat packet would come in an hit the pool and return faster than the open
// (returning a file-not-found error).
// The below by itself wouldn't trigger the race however, I needed to add a
// small sleep in the openpacket code to trigger the issue. I wanted to add a
// way to inject that in the code but right now there is no good place for it.
// I'm thinking after I convert the server into a request-server backend I
// might be able to do something with the runWorker method passed into the
// packet manager. But with the 2 implementations fo the server it just doesn't
// fit well right now.
func TestOpenStatRace(t *testing.T) {
	client, server := clientServerPair(t)
	defer client.Close()
	defer server.Close()

	// openpacket finishes to fast to trigger race in tests
	// need to add a small sleep on server to openpackets somehow
	tmppath := path.Join(os.TempDir(), "stat_race")
	pflags := flags(os.O_RDWR | os.O_CREATE | os.O_TRUNC)
	ch := make(chan result, 3)
	id1 := client.nextID()
	client.dispatchRequest(ch, &sshFxpOpenPacket{
		ID:     id1,
		Path:   tmppath,
		Pflags: pflags,
	})
	id2 := client.nextID()
	client.dispatchRequest(ch, &sshFxpLstatPacket{
		ID:   id2,
		Path: tmppath,
	})
	testreply := func(id uint32) {
		r := <-ch
		switch r.typ {
		case sshFxpAttrs, sshFxpHandle: // ignore
		case sshFxpStatus:
			err := normaliseError(unmarshalStatus(id, r.data))
			assert.NoError(t, err, "race hit, stat before open")
		default:
			t.Fatal("unexpected type:", r.typ)
		}
	}
	testreply(id1)
	testreply(id2)
	os.Remove(tmppath)
	checkServerAllocator(t, server)
}

// Ensure that proper error codes are returned for non existent files, such
// that they are mapped back to a 'not exists' error on the client side.
func TestStatNonExistent(t *testing.T) {
	client, server := clientServerPair(t)
	defer client.Close()
	defer server.Close()

	for _, file := range []string{"/doesnotexist", "/doesnotexist/a/b"} {
		_, err := client.Stat(file)
		if !os.IsNotExist(err) {
			t.Errorf("expected 'does not exist' err for file %q.  got: %v", file, err)
		}
	}
}

func TestServerWithBrokenClient(t *testing.T) {
	validInit := sp(&sshFxInitPacket{Version: 3})
	brokenOpen := sp(&sshFxpOpenPacket{Path: "foo"})
	brokenOpen = brokenOpen[:len(brokenOpen)-2]

	for _, clientInput := range [][]byte{
		// Packet length zero (never valid). This used to crash the server.
		{0, 0, 0, 0},
		append(validInit, 0, 0, 0, 0),

		// Client hangs up mid-packet.
		append(validInit, brokenOpen...),
	} {
		srv, err := NewServer(struct {
			io.Reader
			io.WriteCloser
		}{
			bytes.NewReader(clientInput),
			&sink{},
		})
		require.NoError(t, err)

		err = srv.Serve()
		assert.Error(t, err)
		srv.Close()
	}
}

func TestChroot(t *testing.T) {
	tmpFolder := "/var/tmp"
	if runtime.GOOS == "plan9" {
		tmpFolder = "/tmp"
	} else if runtime.GOOS == "windows" {
		tmpFolder = "C:/Windows/Temp"
	}
	rootPath, err := ioutil.TempDir(tmpFolder, "sftp")
	require.Nil(t, err)
	defer os.RemoveAll(rootPath)

	client, server := clientServerPair(t, Chroot(rootPath))
	defer client.Close()
	defer server.Close()

	t.Run("stat", func(t *testing.T) {
		// prepare, create file and symlink for stat
		require.Nil(t, os.MkdirAll(filepath.Join(rootPath, "/stat"), 0700))
		regular := "/stat/regular"
		symlink := "/stat/symlink"
		content := []byte(strings.Repeat("hello sftp", 1024))
		require.Nil(t, ioutil.WriteFile(filepath.Join(rootPath, regular), content, 0700))
		require.Nil(t, os.Symlink(filepath.Join(rootPath, regular), filepath.Join(rootPath, symlink)))
		t.Run("regular-stat", func(t *testing.T) {
			f, err := client.Stat(regular)
			require.Nil(t, err)
			require.NotNil(t, f)
			assert.EqualValues(t, filepath.Base(regular), f.Name())
			assert.EqualValues(t, len(content), f.Size())
			assert.True(t, f.Mode().IsRegular())
			assert.False(t, f.IsDir())
		})
		t.Run("symlink-stat", func(t *testing.T) {
			f, err := client.Stat(symlink)
			require.Nil(t, err)
			require.NotNil(t, f)
			assert.EqualValues(t, filepath.Base(symlink), f.Name())
			assert.EqualValues(t, len(content), f.Size())
			assert.True(t, f.Mode().IsRegular())
			assert.False(t, f.IsDir())
		})
		t.Run("regular-lstat", func(t *testing.T) {
			f, err := client.Lstat(regular)
			require.Nil(t, err)
			require.NotNil(t, f)
			assert.EqualValues(t, filepath.Base(regular), f.Name())
			assert.EqualValues(t, len(content), f.Size())
			assert.True(t, f.Mode().IsRegular())
			assert.False(t, f.IsDir())
		})
		t.Run("symlink-lstat", func(t *testing.T) {
			f, err := client.Lstat(symlink)
			require.Nil(t, err)
			require.NotNil(t, f)
			assert.EqualValues(t, filepath.Base(symlink), f.Name())
			assert.Greater(t, int64(128), f.Size()) // may have some meta datas
			assert.NotZero(t, f.Mode()&fs.ModeSymlink)
		})
		t.Run("readlink", func(t *testing.T) {
			f, err := client.ReadLink(symlink)
			require.Nil(t, err)
			assert.Equal(t, regular, f)
		})
	})
	t.Run("dir", func(t *testing.T) {
		require.Nil(t, os.MkdirAll(filepath.Join(rootPath, "/dir"), 0700))
		assertDir := func(absPath string, exist bool) {
			f, err := os.Lstat(absPath)
			if !exist {
				require.True(t, os.IsNotExist(err))
			} else {
				require.Nil(t, err)
				assert.True(t, f.IsDir())
			}
		}
		t.Run("mkdir", func(t *testing.T) {
			relPath := "/dir/mkdir"
			require.Nil(t, client.Mkdir(relPath))
			// dir should be created
			assertDir(filepath.Join(rootPath, relPath), true)
			// cannot create nested dir
			require.NotNil(t, client.Mkdir("/dir/mkdir/nested/should/fail"))
		})
		t.Run("mkdirall", func(t *testing.T) {
			relPath := "/dir/mkdir-all/nested"
			require.Nil(t, client.MkdirAll(relPath))
			// dir should be created
			assertDir(filepath.Join(rootPath, relPath), true)
		})
		t.Run("rmdir", func(t *testing.T) {
			// prepare
			relPath := "/dir/rmdir"
			require.Nil(t, os.MkdirAll(filepath.Join(rootPath, relPath), 0700))
			require.Nil(t, ioutil.WriteFile(filepath.Join(rootPath, relPath, "nested"), []byte("some file"), 0700))
			// cannot remove a not empty dir
			require.NotNil(t, client.RemoveDirectory(relPath))
			// remove nested file, to remove an empty dir
			require.Nil(t, os.Remove(filepath.Join(rootPath, relPath, "nested")))
			// call sftp cmd
			require.Nil(t, client.RemoveDirectory(relPath))
			// dir should be removed
			assertDir(filepath.Join(rootPath, relPath), false)
		})
	})
	t.Run("file", func(t *testing.T) {
		require.Nil(t, os.MkdirAll(filepath.Join(rootPath, "/file"), 0700))
		t.Run("symlink", func(t *testing.T) {
			// prepare
			regular := "/file/regular"
			symlink := "/file/symlink"
			content := []byte(strings.Repeat("hello sftp", 1024))
			require.Nil(t, ioutil.WriteFile(filepath.Join(rootPath, regular), content, 0700))
			// call sftp cmd
			require.Nil(t, client.Symlink(regular, symlink))
			// check result, symlink should be created
			f, err := os.Lstat(filepath.Join(rootPath, symlink))
			require.Nil(t, err)
			assert.EqualValues(t, filepath.Base(symlink), f.Name())
			assert.NotZero(t, f.Mode()&fs.ModeSymlink)
		})
		t.Run("rename", func(t *testing.T) {
			// prepare
			oldfile := "/file/oldfile"
			newfile := "/file/newfile"
			content := []byte(strings.Repeat("hello sftp", 1024))
			require.Nil(t, ioutil.WriteFile(filepath.Join(rootPath, oldfile), content, 0700))
			// call sftp cmd
			require.Nil(t, client.Rename(oldfile, newfile))
			// check result, ori file should rename to new
			require.NoFileExists(t, filepath.Join(rootPath, oldfile))
			require.FileExists(t, filepath.Join(rootPath, newfile))
		})
		t.Run("remove", func(t *testing.T) {
			// prepare
			toRemove := "/file/to-remove"
			content := []byte(strings.Repeat("hello sftp", 1024))
			require.Nil(t, ioutil.WriteFile(filepath.Join(rootPath, toRemove), content, 0700))
			// call sftp cmd
			require.Nil(t, client.Remove(toRemove))
			// check result, file should be removed
			require.NoFileExists(t, filepath.Join(rootPath, toRemove))
			// cannot remove file not exist
			require.NotNil(t, client.Remove(toRemove))
		})
		t.Run("open", func(t *testing.T) {
			// prepare
			readfile := "/file/readfile"
			content := []byte(strings.Repeat("hello sftp", 1024))
			require.Nil(t, ioutil.WriteFile(filepath.Join(rootPath, readfile), content, 0700))
			// call sftp cmd
			f, err := client.Open(readfile)
			require.Nil(t, err)
			require.NotNil(t, f)
			defer f.Close()
			bytes, err := ioutil.ReadAll(f)
			// check result
			require.Nil(t, err)
			assert.EqualValues(t, content, bytes)
		})
		t.Run("write", func(t *testing.T) {
			// prepare
			writefile := "/file/writefile"
			content := []byte(strings.Repeat("hello sftp", 1024))
			// call sftp cmd
			f, err := client.Create(writefile)
			require.Nil(t, err)
			require.NotNil(t, f)
			defer f.Close()
			n, err := f.Write(content)
			require.Nil(t, err)
			assert.EqualValues(t, len(content), n)
			// check result
			require.FileExists(t, filepath.Join(rootPath, writefile))
			bytes, err := ioutil.ReadFile(filepath.Join(rootPath, writefile))
			require.Nil(t, err)
			assert.EqualValues(t, content, bytes)
		})
	})
	t.Run("relative", func(t *testing.T) {
		require.Nil(t, os.MkdirAll(filepath.Join(rootPath, "/relative"), 0700))
		t.Run("opendir", func(t *testing.T) {
			require.Nil(t, ioutil.WriteFile(filepath.Join(rootPath, "/relative/file1"), []byte("file1"), 0700))
			require.Nil(t, ioutil.WriteFile(filepath.Join(rootPath, "/relative/file2"), []byte("file2"), 0700))
			files, err := client.ReadDir("/relative")
			require.Nil(t, err)
			require.Len(t, files, 2)
			for _, file := range files {
				assert.Contains(t, file.Name(), "file")
				assert.EqualValues(t, file.Size(), 5)
			}
		})
	})
	t.Run("realpath", func(t *testing.T) {
		f, err := client.RealPath(".")
		require.Nil(t, err)
		assert.Equal(t, "/", f)
	})
	t.Run("out-of-path", func(t *testing.T) {
		_, err := client.RealPath("..")
		require.NotNil(t, err)
		_, err = client.ReadDir("..")
		require.NotNil(t, err)
	})
}
