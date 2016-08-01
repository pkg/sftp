sftp
----

The `sftp` package provides support for file system operations on remote ssh servers using the SFTP subsystem.

This is a fork of [github.com/pkg/sftp](http://github.com/pkg/sftp) that changes the server to allow for a plugable backend and adds an
S3 and file system backend. The file system backend is mostly used for tests and is similar to the previous behavior. Additionally, this
adds a ManagedServer component that can be used to easily create a server with an arbitrary backend.

Tests that depend on the details of the file system server (mostly client tests) are being skipped.

usage and examples
------------------

See [godoc.org/github.com/pkg/sftp](http://godoc.org/github.com/pkg/sftp) for examples and usage.

The basic operation of the package mirrors the facilities of the [os](http://golang.org/pkg/os) package.

The Walker interface for directory traversal is heavily inspired by Keith Rarick's [fs](http://godoc.org/github.com/kr/fs) package.

roadmap
-------

 * There is way too much duplication in the Client methods. If there was an unmarshal(interface{}) method this would reduce a heap of the duplication.

contributing
------------

We welcome pull requests, bug fixes and issue reports.

Before proposing a large change, first please discuss your change by raising an issue.
