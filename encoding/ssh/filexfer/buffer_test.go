package sshfx

import (
	"testing"
)

func BenchmarkAppendCount(b *testing.B) {
	buf := NewBuffer(make([]byte, 0, b.N*4))

	for i := range b.N {
		buf.AppendCount(i)
	}
}

func BenchmarkAppendString(b *testing.B) {
	buf := NewBuffer(make([]byte, 0, b.N*(4+3)))

	for range b.N {
		buf.AppendString("foo")
	}
}
