package pool

import (
	"errors"
	"sync"
	"sync/atomic"
)

type metrics struct{
	hits atomic.Uint64
	misses atomic.Uint64
}

func (m *metrics) hit() {
	m.hits.Add(1)
}

func (m *metrics) miss() {
	m.misses.Add(1)
}

func (m *metrics) Hits() (hits, total uint64) {
	hits = m.hits.Load()
	return hits, hits + m.misses.Load()
}

// BufPool provides a pool of slices that will return nil when a miss occurs.
type SlicePool[S []T, T any] struct{
	metrics

	ch     chan S
	length int
}

func NewSlicePool[S []T, T any](depth, cullLength int) *SlicePool[S,T] {
	if cullLength <= 0 {
		panic("sftp: bufPool: new buffer creation length must be greater than zero")
	}

	return &SlicePool[S,T]{
		ch:     make(chan S, depth),
		length: cullLength,
	}
}

func (p *SlicePool[S,T]) Get() S {
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

func (p *SlicePool[S,T]) Put(b S) {
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

// Pool provides a pool of types that should be called with new(T) when a miss occurs.
type Pool[T any] struct{
	metrics

	ch     chan *T
}

func NewPool[T any](depth int) *Pool[T] {
	return &Pool[T]{
		ch:     make(chan *T, depth),
	}
}

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

// WorkPool provides a pool of types that blocks when the pool is empty.
type WorkPool[T any] struct{
	ch chan chan T
	wg sync.WaitGroup
}

func NewWorkPool[T any](depth int) *WorkPool[T] {
	p := &WorkPool[T]{
		ch:   make(chan chan T, depth),
	}

	for len(p.ch) < cap(p.ch) {
		p.ch <- make(chan T, 1)
	}

	return p
}

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
