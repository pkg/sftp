package sync

import (
	"errors"
	"sync"

	"github.com/pkg/sftp/v2/internal/pragma"
)

// SlicePool is a set of temporary slices that may be individually saved and retrieved.
// It is intended to mirror [sync.Pool], except it has been specifically designed to meet the needs of pkg/sftp.
//
// Any slice stored in the SlicePool will be held onto indefinitely,
// and slices are returned for reuse in a round-robin order.
//
// A SlicePool is safe for use by multiple goroutines simultaneously.
//
// SlicePool's purpose is to cache allocated but unused slices for later reuse,
// relieving pressure on the garbage collector and amortizing allocation overhead.
//
// Unlike the standard library Pool, it is suitable to act as a free list of short-lived slices,
// since the free list is maintained as a channel, and thus has fairly low overhead.
type SlicePool[S []T, T any] struct {
	noCopy pragma.DoNotCopy

	metrics

	ch     chan S
	length int
}

// NewSlicePool returns a [SlicePool] set to hold onto depth number of items,
// and discard any slice with a capacity greater than the cull length.
//
// It will panic if given a negative depth, the same as making a negative-buffer channel.
// It will also panic if given a zero or negative cull length.
func NewSlicePool[S []T, T any](depth, cullLength int) *SlicePool[S, T] {
	if cullLength <= 0 {
		panic("sftp: bufPool: new buffer creation length must be greater than zero")
	}

	return &SlicePool[S, T]{
		ch:     make(chan S, depth),
		length: cullLength,
	}
}

// Get retrieves a slice from the pool, sets the length to the capacity, and then returns it to the caller.
// If the pool is empty, it will return a nil slice.
//
// A nil SlicePool is treated as an empty pool,
// that is, it returns only nil slices.
func (p *SlicePool[S, T]) Get() S {
	if p == nil {
		return nil
	}

	select {
	case b := <-p.ch:
		p.hit()
		return b[:cap(b)] // re-extend to the full length.

	default:
		p.miss()
		return nil // Don't over allocate; let ReadFrom allocate the specific size.
	}
}

// Put adds the slice to the pool, if there is capacity in the pool,
// and if the capacity of the slice is less than the culling length.
//
// A nil SlicePool is treated as a pool with no capacity.
func (p *SlicePool[S, T]) Put(b S) {
	if p == nil {
		// functional default: no reuse
		return
	}

	if cap(b) > p.length {
		// DO NOT reuse buffers with excessive capacity.
		// This could cause memory leaks.
		return
	}

	select {
	case p.ch <- b:
	default:
	}
}

// Pool is a set of temporary items that may be individually saved and retrieved.
// It is intended to mirror [sync.Pool], except it has been specifically designed to meet the needs of pkg/sftp.
//
// Any item stored in the Pool will be held onto indefinitely,
// and items are returned for reuse in a round-robin order.
//
// A Pool is safe for use by multiple goroutines simultaneously.
//
// Pool's purpose is to cache allocated but unused items for later reuse,
// relieving pressure on the garbage collector and amortizing allocation overhead.
//
// Unlike the standard library Pool, it is suitable to act as a free list of short-lived items,
// since the free list is maintained as a channel, and thus has fairly low overhead.
type Pool[T any] struct {
	noCopy pragma.DoNotCopy

	metrics

	ch chan *T
}

// NewPool returns a [Pool] set to hold onto depth number of pointers to the given type.
//
// It will panic if given a negative depth, the same as making a negative-buffer channel.
func NewPool[T any](depth int) *Pool[T] {
	return &Pool[T]{
		ch: make(chan *T, depth),
	}
}

// Get retrieves an item from the pool, and then returns it to the caller.
// If the pool is empty, it will return a pointer to a newly allocated item.
//
// A nil Pool is treated as an empty pool,
// that is, it always returns a pointer to a newly allocated item.
func (p *Pool[T]) Get() *T {
	if p == nil {
		return new(T)
	}

	select {
	case v := <-p.ch:
		p.hit()
		return v

	default:
		p.miss()
		return new(T)
	}
}

// Put adds the given pointer to item to the pool, if there is capacity in the pool.
//
// A nil Pool is treated as a pool with no capacity.
func (p *Pool[T]) Put(v *T) {
	if p == nil {
		// functional default: no reuse
		return
	}

	var z T
	*v = z // shallow zero.

	select {
	case p.ch <- v:
	default:
	}
}

// WorkPool is a set of temporary work channels that can co-ordinate returns of work done among goroutines.
// It is intended to mimic [sync.Pool], except it has been specifically designed to meet the needs of pkg/sftp.
//
// A WorkPool will be filled to capacity at creation with work channels of the given type and a buffer of 1.
// It will track channels that have been handed out through Get,
// blocking on Close until all of them have been returned.
//
// WorkPool's purpose is also to block allocate work channels for reuse during concurrent transfers,
// relieving pressure on the garbage collector and amortizing allocation overhead.
// While also co-ordinating outstanding work, so the caller can wait for all work to be complete.
type WorkPool[T any] struct {
	wg sync.WaitGroup

	ch chan chan T
}

// NewWorkPool returns a [WorkPool] set to hold onto depth number of channels of the given type.
//
// It will panic if given a negative depth, the same as making a negative-buffer channel.
func NewWorkPool[T any](depth int) *WorkPool[T] {
	p := &WorkPool[T]{
		ch: make(chan chan T, depth),
	}

	for len(p.ch) < cap(p.ch) {
		p.ch <- make(chan T, 1)
	}

	return p
}

// Close closes the [WorkPool] to all further Get request.
// Close then waits for all outstanding channels to be returned to the pool.
//
// After calling Close, all calls to Get will return a nil work channel and false.
//
// After Close returns, the pool will be empty,
// and all work channels will have been discarded and ready for the garbage collector.
//
// It is an error not a panic to close a nil WorkPool.
// However, Close will panic if called more than once.
func (p *WorkPool[T]) Close() error {
	if p == nil {
		return errors.New("cannot close nil work pool")
	}

	close(p.ch)

	p.wg.Wait()

	for range p.ch {
		// drain the pool and drop them on all on the ground for GC.
	}

	return nil
}

// Get retrieves a work channel from the pool, and then returns it to the caller,
// or it returns a nil channel, and false if the [WorkPool] has been closed.
//
// If no work channels are available, it will block until a work channel has been returned to the pool.
//
// A nil WorkPool will simply always return a new work channel and true.
func (p *WorkPool[T]) Get() (chan T, bool) {
	if p == nil {
		return make(chan T, 1), true
	}

	v, ok := <-p.ch
	if ok {
		p.wg.Add(1)
	}
	return v, ok
}

// Put returns the given work channel to the pool.
//
// Put panics if an attempt is made to return more work channels to the pool than the capacity of the pool.
//
// A nil SlicePool will simply discard work channels.
func (p *WorkPool[T]) Put(v chan T) {
	if p == nil {
		// functional default: no reuse
		return
	}

	select {
	case p.ch <- v:
		p.wg.Done()
	default:
		panic("worker pool overfill")
		// This is an overfill, which shouldn't happen, but just in case...
	}
}
