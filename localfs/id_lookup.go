package localfs

import (
	"os/user"
)

// LookupUserName returns the OS username for the given uid.
func (*ServerHandler) LookupUserName(uid string) string {
	u, err := user.LookupId(uid)
	if err != nil {
		return uid
	}

	return u.Username
}

// LookupGroupName returns the OS group name for the given gid.
func (*ServerHandler) LookupGroupName(gid string) string {
	g, err := user.LookupGroupId(gid)
	if err != nil {
		return gid
	}

	return g.Name
}
