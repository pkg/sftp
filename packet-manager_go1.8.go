// +build go1.8

package sftp

import "sort"

type responsePackets []responsePacket

func (r responsePackets) Sort() {
	sort.Slice(r, func(i, j int) bool {
		return r[i].id() < r[j].id()
	})
}
