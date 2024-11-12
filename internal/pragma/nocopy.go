package pragma

// DoNotCopy may be added to structs which must not be copied after first use.
//
// See https://golang.org/issues/8005#issuecomment-190753527 for details
type DoNotCopy struct{}

// Lock is a no-op used by -copylocks checker from `go vet`.
func (*DoNotCopy) Lock() {}

// Unlock is a no-op used by -copylocks checker from `go vet`.
func (*DoNotCopy) Unlock() {}
