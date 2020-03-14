package sftp

import (
	"sync"
)

type allocator struct {
	available [][]byte
	// map key is the request order
	used map[uint32][][]byte
	sync.Mutex
}

func newAllocator() *allocator {
	return &allocator{
		available: nil,
		used:      make(map[uint32][][]byte),
	}
}

// GetPage returns a previously allocated and unused []byte or create a new one.
// The slice have a fixed size = maxMsgLength, this value is suitable for both
// receiving new packets and reading the files to serve
func (a *allocator) GetPage(requestOrderID uint32) []byte {
	a.Lock()
	defer a.Unlock()

	var result []byte

	// get an available page and remove it from the available ones
	if len(a.available) > 0 {
		truncLength := len(a.available) - 1
		result = a.available[truncLength]

		a.available[truncLength] = nil          // clear out the internal pointer
		a.available = a.available[:truncLength] // truncate the slice
	}

	// no preallocated slice found, just allocate a new one
	if result == nil {
		result = make([]byte, maxMsgLength)
	}

	// put result in used pages
	a.used[requestOrderID] = append(a.used[requestOrderID], result)

	return result
}

// ReleasePages marks unused all pages in use for the given requestID
func (a *allocator) ReleasePages(requestOrderID uint32) {
	a.Lock()
	defer a.Unlock()

	if used, ok := a.used[requestOrderID]; ok && len(used) > 0 {
		a.available = append(a.available, used...)
		// this is probably useless
		a.used[requestOrderID] = nil
	}
	delete(a.used, requestOrderID)
}

// Free removes all the used and free pages.
// Call this method when the allocator is not needed anymore
func (a *allocator) Free() {
	a.Lock()
	defer a.Unlock()

	a.available = nil
	a.used = make(map[uint32][][]byte)
}

func (a *allocator) countUsedPages() int {
	a.Lock()
	defer a.Unlock()

	num := 0
	for _, p := range a.used {
		num += len(p)
	}
	return num
}

func (a *allocator) countAvailablePages() int {
	a.Lock()
	defer a.Unlock()

	return len(a.available)
}

func (a *allocator) isRequestOrderIDUsed(requestOrderID uint32) bool {
	a.Lock()
	defer a.Unlock()

	if _, ok := a.used[requestOrderID]; ok {
		return true
	}
	return false
}
