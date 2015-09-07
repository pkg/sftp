// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build darwin dragonfly freebsd !android,linux netbsd openbsd solaris
// +build cgo

package user

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

/*
#include <unistd.h>
#include <sys/types.h>
#include <pwd.h>
#include <grp.h>
#include <stdlib.h>

static int mygetpwuid_r(int uid, struct passwd *pwd,
	char *buf, size_t buflen, struct passwd **result) {
 return getpwuid_r(uid, pwd, buf, buflen, result);
}

static int mygetgrgid_r(int gid, struct group *grp,
	char *buf, size_t buflen, struct group **result) {
 return getgrgid_r(gid, grp, buf, buflen, result);
}

static int mygetgrouplist(const char *user, gid_t group, gid_t *groups,
	int *ngroups) {
 return getgrouplist(user, group, (void *)groups, ngroups);
}

static inline gid_t group_at(int i, gid_t *groups) {
 return groups[i];
}

static inline char **next_member(char **members) { return members + 1; }

*/
import "C"

const (
	userBuffer = iota
	groupBuffer
)

func current() (*User, error) {
	return lookupUnixUser(syscall.Getuid(), "", false)
}

func lookupUser(username string) (*User, error) {
	return lookupUnixUser(-1, username, true)
}

func lookupUserId(uid string) (*User, error) {
	i, e := strconv.Atoi(uid)
	if e != nil {
		return nil, e
	}
	return lookupUnixUser(i, "", false)
}

func lookupUnixUser(uid int, username string, lookupByName bool) (*User, error) {
	var pwd C.struct_passwd
	var result *C.struct_passwd

	var bufSize C.long
	if runtime.GOOS == "dragonfly" || runtime.GOOS == "freebsd" {
		// DragonFly and FreeBSD do not have _SC_GETPW_R_SIZE_MAX
		// and just return -1.  So just use the same
		// size that Linux returns.
		bufSize = 1024
	} else {
		bufSize = C.sysconf(C._SC_GETPW_R_SIZE_MAX)
		if bufSize <= 0 || bufSize > 1<<20 {
			return nil, fmt.Errorf("user: unreasonable _SC_GETPW_R_SIZE_MAX of %d", bufSize)
		}
	}
	buf := C.malloc(C.size_t(bufSize))
	defer C.free(buf)
	var rv C.int
	if lookupByName {
		nameC := C.CString(username)
		defer C.free(unsafe.Pointer(nameC))
		rv = C.getpwnam_r(nameC,
			&pwd,
			(*C.char)(buf),
			C.size_t(bufSize),
			&result)
		if rv != 0 {
			return nil, fmt.Errorf("user: lookup username %s: %s", username, syscall.Errno(rv))
		}
		if result == nil {
			return nil, UnknownUserError(username)
		}
	} else {
		// mygetpwuid_r is a wrapper around getpwuid_r to
		// to avoid using uid_t because C.uid_t(uid) for
		// unknown reasons doesn't work on linux.
		rv = C.mygetpwuid_r(C.int(uid),
			&pwd,
			(*C.char)(buf),
			C.size_t(bufSize),
			&result)
		if rv != 0 {
			return nil, fmt.Errorf("user: lookup userid %d: %s", uid, syscall.Errno(rv))
		}
		if result == nil {
			return nil, UnknownUserIdError(uid)
		}
	}
	u := &User{
		Uid:      strconv.Itoa(int(pwd.pw_uid)),
		Gid:      strconv.Itoa(int(pwd.pw_gid)),
		Username: C.GoString(pwd.pw_name),
		Name:     C.GoString(pwd.pw_gecos),
		HomeDir:  C.GoString(pwd.pw_dir),
	}
	// The pw_gecos field isn't quite standardized.  Some docs
	// say: "It is expected to be a comma separated list of
	// personal data where the first item is the full name of the
	// user."
	if i := strings.Index(u.Name, ","); i >= 0 {
		u.Name = u.Name[:i]
	}
	return u, nil
}

func currentGroup() (*Group, error) {
	return lookupUnixGroup(syscall.Getgid(), "", false, buildGroup)
}

func lookupGroup(groupname string) (*Group, error) {
	return lookupUnixGroup(-1, groupname, true, buildGroup)
}

func lookupGroupId(gid string) (*Group, error) {
	i, e := strconv.Atoi(gid)
	if e != nil {
		return nil, e
	}
	return lookupUnixGroup(i, "", false, buildGroup)
}

func lookupUnixGroup(gid int, groupname string, lookupByName bool, f func(*C.struct_group) *Group) (*Group, error) {
	var grp C.struct_group
	var result *C.struct_group

	buf, bufSize, err := allocBuffer(groupBuffer)
	if err != nil {
		return nil, err
	}
	defer C.free(buf)

	if lookupByName {
		nameC := C.CString(groupname)
		defer C.free(unsafe.Pointer(nameC))
		rv := C.getgrnam_r(nameC,
			&grp,
			(*C.char)(buf),
			C.size_t(bufSize),
			&result)
		if rv != 0 {
			return nil, fmt.Errorf("group: lookup groupname %s: %s", groupname, syscall.Errno(rv))
		}
		if result == nil {
			return nil, UnknownGroupError(groupname)
		}
	} else {
		// mygetgrgid_r is a wrapper around getgrgid_r to
		// to avoid using gid_t because C.gid_t(gid) for
		// unknown reasons doesn't work on linux.
		rv := C.mygetgrgid_r(C.int(gid),
			&grp,
			(*C.char)(buf),
			C.size_t(bufSize),
			&result)
		if rv != 0 {
			return nil, fmt.Errorf("group: lookup groupid %d: %s", gid, syscall.Errno(rv))
		}
		if result == nil {
			return nil, UnknownGroupIdError(gid)
		}
	}
	g := f(&grp)
	return g, nil
}

func buildGroup(grp *C.struct_group) *Group {
	g := &Group{
		Gid:  strconv.Itoa(int(grp.gr_gid)),
		Name: C.GoString(grp.gr_name),
	}
	return g
}

func userInGroup(u *User, g *Group) (bool, error) {
	if u.Gid == g.Gid {
		return true, nil
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return false, err
	}

	nameC := C.CString(u.Username)
	defer C.free(unsafe.Pointer(nameC))
	groupC := C.gid_t(gid)
	ngroupsC := C.int(0)

	C.mygetgrouplist(nameC, groupC, nil, &ngroupsC)
	ngroups := int(ngroupsC)

	groups := C.malloc(C.size_t(int(unsafe.Sizeof(groupC)) * ngroups))
	defer C.free(groups)

	rv := C.mygetgrouplist(nameC, groupC, (*C.gid_t)(groups), &ngroupsC)
	if rv == -1 {
		return false, fmt.Errorf("user: membership of %s in %s: %s", u.Username, g.Name, syscall.Errno(rv))
	}

	ngroups = int(ngroupsC)
	for i := 0; i < ngroups; i++ {
		gid := C.group_at(C.int(i), (*C.gid_t)(groups))
		if g.Gid == strconv.Itoa(int(gid)) {
			return true, nil
		}
	}
	return false, nil
}

func groupMembers(g *Group) ([]string, error) {
	var members []string
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return nil, err
	}

	_, err = lookupUnixGroup(gid, "", false, func(grp *C.struct_group) *Group {
		cmem := grp.gr_mem
		for *cmem != nil {
			members = append(members, C.GoString(*cmem))
			cmem = C.next_member(cmem)
		}
		return g
	})
	if err != nil {
		return nil, err
	}

	return members, nil
}

func allocBuffer(bufType int) (unsafe.Pointer, C.long, error) {
	var bufSize C.long
	if runtime.GOOS == "freebsd" {
		// FreeBSD doesn't have _SC_GETPW_R_SIZE_MAX
		// or SC_GETGR_R_SIZE_MAX and just returns -1.
		// So just use the same size that Linux returns
		bufSize = 1024
	} else {
		var size C.int
		var constName string
		switch bufType {
		case userBuffer:
			size = C._SC_GETPW_R_SIZE_MAX
			constName = "_SC_GETPW_R_SIZE_MAX"
		case groupBuffer:
			size = C._SC_GETGR_R_SIZE_MAX
			constName = "_SC_GETGR_R_SIZE_MAX"
		}
		bufSize = C.sysconf(size)
		if bufSize <= 0 || bufSize > 1<<20 {
			return nil, bufSize, fmt.Errorf("user: unreasonable %s of %d", constName, bufSize)
		}
	}
	return C.malloc(C.size_t(bufSize)), bufSize, nil
}
