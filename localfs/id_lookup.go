package localfs

import (
	"os/user"
)

func (*ServerHandler) LookupUserName(uid string) string {
	u, err := user.LookupId(uid)
	if err != nil {
		return uid
	}

	return u.Username
}

func (*ServerHandler) LookupGroupName(gid string) string {
	g, err := user.LookupGroupId(gid)
	if err != nil {
		return gid
	}

	return g.Name
}
