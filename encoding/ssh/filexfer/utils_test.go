package sshfx

import (
	"testing"
)

func expectAllocs(t *testing.T, count int, f func()) {
	t.Helper()

	allocs := testing.AllocsPerRun(10, f)
	if allocs != float64(count) {
		t.Errorf("unexpected number of allocs %.0f, was expected %d", allocs, count)
	}
}

func expectOnlyOneAlloc(t *testing.T, f func()) {
	t.Helper()

	expectAllocs(t, 1, f)
}

type marshalPacketFunc func(reqid uint32, b []byte) (header, payload []byte, err error)

func testComposePacket(t *testing.T, marshal marshalPacketFunc, reqid uint32, b []byte) (data []byte, err error) {
	t.Helper()

	expectOnlyOneAlloc(t, func() {
		_, _, _ = marshal(reqid, b)
	})

	return ComposePacket(marshal(reqid, b))
}
