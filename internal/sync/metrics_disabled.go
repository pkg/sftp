//go:build !sftp.sync.metrics

package sync

// metrics no-opss hit and miss metrics.
type metrics struct{}

func (m *metrics) hit() {}

func (m *metrics) miss() {}

// Hits always returns 0, 0.
// To enable tracking metrics, include the build tag "sftp.sync.metrics".
func (m *metrics) Hits() (hits, total uint64) {
	return 0, 0
}
