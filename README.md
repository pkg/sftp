sftp
----

The `sftp` package provides support for file system operations on remote ssh servers using the SFTP subsystem.
It also implements an SFTP server for serving files from the local filesystem.

![CI Status](https://github.com/pkg/sftp/workflows/CI/badge.svg?branch=master&event=push) [![Go Reference](https://pkg.go.dev/badge/github.com/pkg/sftp/v2.svg)](https://pkg.go.dev/github.com/pkg/sftp/v2)

usage and examples
------------------

See [https://pkg.go.dev/github.com/pkg/sftp/v2](https://pkg.go.dev/github.com/pkg/sftp/v2) for examples and usage.

The basic operation of the client mirrors the facilities of the [os](http://pkg.go.dev/os) package.

The basic interface of the server handler follows the design of [gRPC](https://pkg.go.dev/google.golang.org/grpc).


roadmap
-------

* Extensive testing is necessary to validate that functionality has not been lost.

contributing
------------

We welcome pull requests, bug fixes and issue reports.

Before proposing a large change, first please discuss your change by raising an issue.

For API/code bugs, please include a small, self contained code example to reproduce the issue.
For pull requests, remember test coverage.

We try to handle issues and pull requests with a 0 open philosophy.
That means we will try to address the submission as soon as possible and will work toward a resolution.
If progress can no longer be made (eg. unreproducible bug) or stops (eg. unresponsive submitter), we will close the bug.

Thanks.
