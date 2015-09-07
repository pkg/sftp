// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !cgo,!windows,!plan9 android

package user

import (
	"fmt"
	"runtime"
)

func init() {
	implemented = false
}

func current() (*User, error) {
	return nil, fmt.Errorf("user: Current not implemented on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func lookupUser(username string) (*User, error) {
	return nil, fmt.Errorf("user: Lookup not implemented on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func lookupUserId(uid string) (*User, error) {
	return nil, fmt.Errorf("user: LookupId not implemented on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func lookupGroup(groupname string) (*Group, error) {
	return nil, fmt.Errorf("user: LookupGroup not implemented on %s/%s", runtime.GOOS, runtime.GOARCH)
}

func lookupGroupId(int) (*Group, error) {
	return nil, fmt.Errorf("user: LookupGroupId not implemented on %s/%s", runtime.GOOS, runtime.GOARCH)
}
