package sftp

import "testing"

func clientRequestServerPair(t *testing.T) *csPair {
	// We skip these tests because running them on windows exposes other underlying
	// issues with path handling. This allows the package to compile and test
	// what can be tested on windows.
	t.Skip("no socket pair")
}
