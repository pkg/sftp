# Request Based SFTP API

The request based API allows for custom backends in a way similar to the http
package. In order to create a backend you need to implement 4 handler
interfaces; one for reading, one for writing, one for misc commands and one for
listing files. Each has 1 required method and in each case those methods take
the Request as the only parameter and they each return something different.
These 4 interfaces are enough to handle all the SFTP traffic in a simplified
manner.

Unlike `Server`, which serves the local filesystem with the permissions of the
user the process runs as, the `RequestServer` only does what your handlers do.
That makes it the better choice whenever the server does not run as the
connecting user (for example a server listening on port 22 as root), since
your handlers decide what each client can see and change.

## Getting started

Fill in a `Handlers` struct and pass it to `NewRequestServer` together with the
SSH channel (or any other `io.ReadWriteCloser`), then call `Serve`:

```go
server := sftp.NewRequestServer(channel, sftp.Handlers{
	FileGet:  h,
	FilePut:  h,
	FileCmd:  h,
	FileList: h,
})
if err := server.Serve(); err != nil && err != io.EOF {
	log.Print("sftp server completed with error: ", err)
}
server.Close()
```

`InMemHandler` (in request-example.go) is a complete in-memory implementation
of all the handlers and a good reference for writing your own, and
examples/request-server shows how to serve it over SSH.

`NewRequestServer` also takes options: `WithStartDirectory` sets the directory
that relative paths are resolved against, `WithRSAllocator` enables the packet
allocator and `WithRSMaxTxPacket` raises the maximum data length of a read.

## The Request

Each handler gets a `*Request` as its only argument. The fields you will deal
with are:

- Method (string) - name of the incoming call, see the handlers below
- Filepath (string) - cleaned, absolute POSIX path of the file to act on
- Flags (uint32) - file open flags for "Get", "Put" and "Open", and setstat flags for "Setstat"
- Attrs ([]byte) - file attributes in their wire format
- Target (string) - target path for "Rename", "PosixRename", "Link" and "Symlink"

Rather than decoding `Flags` and `Attrs` yourself, use the helper methods:

- `Pflags()` returns the open flags (Read, Write, Append, Creat, Trunc, Excl)
- `AttrFlags()` returns which attributes were sent (Size, UidGid, Permissions, Acmodtime)
- `Attributes()` returns the attributes as a `*FileStat`
- `Context()` returns a context that is canceled when the request is done

See the [SFTP spec](https://datatracker.ietf.org/doc/html/draft-ietf-secsh-filexfer-02#section-6.3)
for the meaning of the open flags.

## Handlers

Below are the required methods and a brief description of what they need to do.

### Fileread(*Request) (io.ReaderAt, error)

Handler for the "Get" method. Returns an `io.ReaderAt` for the file, which the
server then reads from to answer the client's read requests.

### Filewrite(*Request) (io.WriterAt, error)

Handler for the "Put" method, and for "Open" when `OpenFileWriter` is not
implemented. Returns an `io.WriterAt` for the file, which the server then
writes the uploaded data to. When the request has the Append flag, do not open
the file with `O_APPEND` if you use `WriteAt`, as they conflict.

### Filecmd(*Request) error

Handles the "Setstat", "Rename", "Rmdir", "Mkdir", "Link", "Symlink" and
"Remove" methods. Makes the appropriate changes and returns nil for success or
an error. Unless `PosixRenameFileCmder` is implemented, "PosixRename" requests
also arrive here, as "Rename".

### Filelist(*Request) (ListerAt, error)

Handles the "List", "Stat" and "Readlink" methods. Unless `LstatFileLister` is
implemented, "Lstat" requests also arrive here, as "Stat". Returns a `ListerAt`, which works for file
lists like `io.ReaderAt` does for files: the server passes in a
`[]os.FileInfo` buffer and an offset, and `ListAt` copies entries into it and
returns how many it copied, with `io.EOF` once the end of the list is reached.
For "Stat", "Lstat" and "Readlink" the list has a single entry.

If the returned reader, writer or `ListerAt` also implements `io.Closer`, the
server calls `Close` when the client closes the handle.

## Optional interfaces

The handlers can implement these interfaces to support more of the protocol:

- `OpenFileWriter` (FilePut): `OpenFile(*Request) (WriterAtReaderAt, error)`
  handles "Open", so that a client can read and write through the same handle.
- `PosixRenameFileCmder` (FileCmd): `PosixRename(*Request) error` handles
  "PosixRename", which must replace an existing target.
- `StatVFSFileCmder` (FileCmd): `StatVFS(*Request) (*StatVFS, error)` handles
  "StatVFS" for the statvfs@openssh.com extension. Without it, those requests
  fail with SSH_FX_OP_UNSUPPORTED.
- `LstatFileLister` (FileList): `Lstat(*Request) (ListerAt, error)` handles
  "Lstat", which must not follow a final symlink.
- `RealPathFileLister` (FileList): `RealPath(string) (string, error)` replaces
  the built-in path cleaning, for example to resolve symlinks. It must return
  an absolute POSIX path.
- `ReadlinkFileLister` (FileList): `Readlink(string) (string, error)` handles
  "Readlink", since `os.FileInfo.Name` can only return a base name.
- `NameLookupFileLister` (FileList): `LookupUserName(string) string` and
  `LookupGroupName(string) string` are used to show user and group names in
  directory listings.
- `TransferError` (on a returned reader or writer): `TransferError(error)` is
  called with the error that made `Serve` exit while the handle was still open.

Extensions are announced with `SetSFTPExtensions`. By default hardlink@openssh.com,
posix-rename@openssh.com and statvfs@openssh.com are all announced.

## Errors

The error a handler returns is sent to the client as an SFTP status. Errors for
which `os.IsNotExist` is true become SSH_FX_NO_SUCH_FILE, system call errors such
as `syscall.EACCES` are translated to the closest status code, and the `ErrSSHFx…`
errors (`ErrSSHFxPermissionDenied`, `ErrSSHFxOpUnsupported` and so on) select a
status code directly. Anything else is sent as SSH_FX_FAILURE, with the error
text as the message.

## TODO

- Add support for API users to see trace/debugging info of what is going on
inside SFTP server.
