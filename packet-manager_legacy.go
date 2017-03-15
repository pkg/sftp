// +build !go1.8

package sftp

import "sort"

// for sorting/ordering incoming/outgoing
type responsePackets []responsePacket

func (r responsePackets) Len() int           { return len(r) }
func (r responsePackets) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r responsePackets) Less(i, j int) bool { return r[i].id() < r[j].id() }
func (r responsePackets) Sort()              { sort.Sort(r) }
