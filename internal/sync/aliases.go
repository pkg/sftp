package sync

import (
	"sync"
)

// Mutex is an alias to [sync.Mutex]
type Mutex = sync.Mutex

// RWMutex is an alias to [sync.RWMutex]
type RWMutex = sync.RWMutex

// WaitGroup is an alias to [sync.WaitGroup]
type WaitGroup = sync.WaitGroup
