package mempool

import (
	"unsafe"
)

// PoolShard contains a reference to an original
// Pool, but with its own separate fast-access ring
// buffer to further reduce Pool's mutex contention.
type PoolShard[T any] struct {
	UnsafePoolShard

	// a pointer back
	// to the original.
	original *Pool[T]
}

func (p *PoolShard[T]) Get() *T {
	if ptr := p.UnsafePoolShard.Get(); ptr != nil {
		return (*T)(ptr)
	}
	var t *T
	if p.original != nil &&
		p.original.New != nil {
		t = p.original.New()
	}
	return t
}

func (p *PoolShard[T]) Put(t *T) {
	if t == nil {
		return
	}
	if p.original != nil &&
		p.original.Reset != nil &&
		!p.original.Reset(t) {
		return
	}
	ptr := unsafe.Pointer(t)
	p.UnsafePoolShard.put(ptr)
}

// Original returns a reference to the shard's origin pool.
func (p *PoolShard[T]) Original() *Pool[T] { return p.original }

// UnsafePoolShard contains a reference to an original
// UnsafePool, but with its own separate fast-access ring
// buffer to reduce the original UnsafePool's mutex contention.
type UnsafePoolShard struct {
	shard_internal
	_ [cache_line_bytes - unsafe.Sizeof(shard_internal{})%cache_line_bytes]byte
}

type shard_internal struct {
	// underlying main pool.
	pool *UnsafePool

	// fast-access ring-buffer of
	// pointers accessible by PID
	// (running goroutine index).
	ring locals_ring
}

func (s *UnsafePoolShard) Get() unsafe.Pointer {
	_, _ = s.ring, s.pool.ring // nil checks before procPin()

	pid := procPin()
	elem, pid := s.ring.local(pid)
	ptr := elem.Swap(nil)

	if ptr != nil {
		procUnpin()
		return ptr
	}

	elem, _ = s.pool.ring.local(pid)
	ptr = elem.Swap(nil)
	procUnpin()

	if ptr != nil {
		return ptr
	}

	s.pool.mutex.Lock()
	ptr = s.pool.pool.Get()
	s.pool.mutex.Unlock()

	return ptr
}

func (s *UnsafePoolShard) Put(ptr unsafe.Pointer) {
	if ptr != nil {
		// nil checks before procPin()
		_, _ = s.ring, s.pool.ring

		// put ptr.
		s.put(ptr)
	}
}

func (s *UnsafePoolShard) put(ptr unsafe.Pointer) {
	pid := procPin()
	elem, pid := s.ring.local(pid)
	ptr = elem.Swap(ptr)

	if ptr == nil {
		procUnpin()
		return
	}

	elem, _ = s.pool.ring.local(pid)
	ptr = elem.Swap(ptr)
	procUnpin()

	if ptr == nil {
		return
	}

	s.pool.mutex.Lock()
	s.pool.pool.put(ptr)
	s.pool.mutex.Unlock()
}

// Original returns a reference to the shard's origin pool.
func (s *UnsafePoolShard) Original() *UnsafePool { return s.pool }

// Release will release resources of this particular shard.
func (s *UnsafePoolShard) Release() { s.ring.clear() }
