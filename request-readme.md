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
That makes it a better choice when the server is not running as the
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

`InMemHandler` (in request-example.go) is an in-memory implementation of all
the handlers and a good reference for writing your own. It has known gaps
where its behavior does not match POSIX semantics. The
`examples/request-server` example shows how to serve it over SSH.

`NewRequestServer` also takes these options:

- `WithStartDirectory` sets the directory that relative paths are resolved
  against.
- `WithRSAllocator` enables an experimental packet allocator that retains
  allocated slices and reuses them for later packets. It can reduce repeated
  allocations when handling packet-heavy workloads.
- `WithRSMaxTxPacket` raises the maximum permitted length, in bytes, of a
  read response payload. The default is 32768 bytes.

## The Request

Each handler gets a `*Request` as its only argument. The fields you will deal
with are:

- Method (string) - the name of the incoming call; see the handlers below.
- Filepath (string) - the cleaned, absolute POSIX path of the file to act on.
- Flags (uint32) - the file open flags for "Get", "Put" and "Open", or the
  setstat flags for "Setstat".
- Attrs ([]byte) - the file attributes in their wire format.
- Target (string) - the target path for "Rename", "PosixRename", "Link" and
  "Symlink".

Rather than decoding `Flags` and `Attrs` yourself, use the helper methods:

- `Pflags()` returns the open flags (Read, Write, Append, Creat, Trunc, Excl).
- `AttrFlags()` returns which attributes were sent (Size, UidGid, Permissions,
  Acmodtime).
- `Attributes()` returns the attributes as a `*FileStat`.
- `Context()` returns a context that is canceled once the request is complete.

The SFTP open flag values are defined by the protocol and are not the same as
the standard library's `os` open flags; consult the
[SFTP spec](https://datatracker.ietf.org/doc/html/draft-ietf-secsh-filexfer-02#section-6.3)
when interpreting them.

## Handlers

Below are the required methods and a brief description of what they need to do.

### Fileread(*Request) (io.ReaderAt, error)

The handler for the "Get" method should return an `io.ReaderAt` for the file,
or a relevant error. The server reads from it to answer the client's read
requests. If the returned reader implements `io.Closer`, the server calls
`Close` when the client closes the file handle.

### Filewrite(*Request) (io.WriterAt, error)

The handler for the "Put" method, and for "Open" when `OpenFileWriter` is not
implemented, should return an `io.WriterAt` for the file, or a relevant error.
The server writes uploaded data to it. Be careful when a request has the Append
flag: opening an `os.File` with `os.O_APPEND` means that its `WriteAt` method
always returns an error. If the returned writer implements `io.Closer`, the
server calls `Close` when the client closes the file handle.

### Filecmd(*Request) error

The handler for "Setstat", "Rename", "Rmdir", "Mkdir", "Link", "Symlink"
and "Remove" should make the requested change and return nil on success or an
error on failure. For "Rename", it should return an error if the target
already exists. "PosixRename" replaces the target atomically when it exists;
when it does not, it behaves like a normal rename. Implement
`PosixRenameFileCmder` to handle that operation. If the optional interface is
not implemented, the server returns `SSH_FX_OP_UNSUPPORTED`.

### Filelist(*Request) (ListerAt, error)

The handler for "List", "Stat", "Lstat" and the fallback implementation of
"Readlink" should return a `ListerAt`, or a relevant error. Unless
`LstatFileLister` is implemented, "Lstat" returns
`SSH_FX_OP_UNSUPPORTED`. A `ListerAt` works for file lists like `io.ReaderAt`
does for files: the server passes in a
`[]os.FileInfo` buffer and an offset, and `ListAt` copies entries into it and
returns how many it copied, with `io.EOF` once the end of the list is reached.
For "Stat", "Lstat" and the fallback "Readlink", it must return exactly one
entry or an error. Without `ReadlinkFileLister`, the fallback sends the
returned `os.FileInfo.Name()` as the link target; implement that optional
interface when the target may include a directory or otherwise needs to be
returned exactly. If the returned `ListerAt` implements `io.Closer`, the
server calls `Close` when the client closes the directory handle.

## Optional interfaces

Handlers can also implement additional optional interfaces to support more of
the protocol:

- `OpenFileWriter` (FilePut): `OpenFile(*Request) (WriterAtReaderAt, error)`
  handles "Open", so that a client can read and write through the same handle.
  If the returned value implements `io.Closer`, the server calls `Close` when
  the client closes the handle.
- `PosixRenameFileCmder` (FileCmd): `PosixRename(*Request) error` handles
  "PosixRename", which must atomically replace an existing target.
- `StatVFSFileCmder` (FileCmd): `StatVFS(*Request) (*StatVFS, error)` handles
  "StatVFS" for the statvfs@openssh.com extension. Without it, those requests
  fail with SSH_FX_OP_UNSUPPORTED.
- `LstatFileLister` (FileList): `Lstat(*Request) (ListerAt, error)` handles
  "Lstat", which must not follow a final symlink. Without it, "Lstat" returns
  `SSH_FX_OP_UNSUPPORTED`.
- `RealPathFileLister` (FileList): `RealPath(string) (string, error)` replaces
  the built-in path cleaning, for example to resolve symlinks. It must return
  an absolute POSIX path.
- `ReadlinkFileLister` (FileList): `Readlink(string) (string, error)` handles
  "Readlink" by returning the symbolic link target. Without it, the fallback
  uses `os.FileInfo.Name()`, which returns only a base name.
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
