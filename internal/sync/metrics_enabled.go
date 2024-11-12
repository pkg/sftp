//go:build sftp.sync.metrics

package sync

import (
	"sync/atomic"
)

// metrics tracks hits and misses for a given pool.
type metrics struct {
	hits   atomic.Uint64
	misses atomic.Uint64
}

func (m *metrics) hit() {
	m.hits.Add(1)
}

func (m *metrics) miss() {
	m.misses.Add(1)
}

// Hits returns a snapshot of hits and misses.
func (m *metrics) Hits() (hits, total uint64) {
	hits = m.hits.Load()
	return hits, hits + m.misses.Load()
}
