package mempool

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/cpu"
)

// Pool provides a form of SimplePool with the
// addition of concurrency safety, and a fast-access
// ring buffer to reduce main mutex contention.
//
// NOTE: the design inherits the
// same caveats as UnsafePool{}.
type Pool[T any] struct {
	UnsafePool

	// New is an optionally provided
	// allocator used when no value
	// is available for use in pool.
	New func() *T

	// Reset is an optionally provided
	// value resetting function called
	// on passed value to Put().
	Reset func(*T) bool
}

func NewPool[T any](new func() *T, reset func(*T) bool, check func(current, victim int) bool) Pool[T] {
	return Pool[T]{
		New:        new,
		Reset:      reset,
		UnsafePool: NewUnsafePool(check),
	}
}

func (p *Pool[T]) Get() *T {
	if ptr := p.UnsafePool.Get(); ptr != nil {
		return (*T)(ptr)
	}
	var t *T
	if p.New != nil {
		t = p.New()
	}
	return t
}

func (p *Pool[T]) Put(t *T) {
	if p.Reset != nil && !p.Reset(t) {
		return
	}
	ptr := unsafe.Pointer(t)
	p.UnsafePool.Put(ptr)
}

// Shard returns a new PoolShard[T] with a reference
// to this original pool. See type for more usage info.
func (p *Pool[T]) Shard() PoolShard[T] {
	return PoolShard[T]{
		original:        p,
		UnsafePoolShard: p.UnsafePool.Shard(),
	}
}

// UnsafePool provides a form of UnsafeSimplePool with
// the addition of concurrency safety, and a fast-access
// ring buffer to reduce main mutex contention.
//
// IMPORTANT NOTE:
// this pool is LOSSY, you cannot guarantee that
// entries entered into the pool will always be
// returned, they may be dropped for GC.
type UnsafePool struct {
	pool_internal
	_ [cache_line_bytes - unsafe.Sizeof(pool_internal{})%cache_line_bytes]byte
}

// Shard returns a new UnsafePoolShard with a reference
// to this original pool. See type for more usage info.
func (p *UnsafePool) Shard() UnsafePoolShard {
	return UnsafePoolShard{shard_internal: shard_internal{
		pool: p,
	}}
}

func NewUnsafePool(check func(current, victim int) bool) UnsafePool {
	return UnsafePool{pool_internal: pool_internal{
		pool: UnsafeSimplePool{Check: check},
	}}
}

const (
	// platform CPU cache line size to avoid false sharing.
	cache_line_bytes = unsafe.Sizeof(cpu.CacheLinePad{})
)

type pool_internal struct {
	// underlying pool and
	// slow mutex protection.
	pool  UnsafeSimplePool
	mutex sync.Mutex

	// fast-access ring-buffer of
	// pointers accessible by PID
	// (running goroutine index).
	ring locals_ring
}

func (p *pool_internal) Check(fn func(current, victim int) bool) func(current, victim int) bool {
	p.mutex.Lock()
	if fn == nil {
		if p.pool.Check == nil {
			fn = defaultCheck
		} else {
			fn = p.pool.Check
		}
	} else {
		p.pool.Check = fn
	}
	p.mutex.Unlock()
	return fn
}

func (p *pool_internal) Get() unsafe.Pointer {
	_ = p.ring // nil check before procPin()

	pid := procPin()
	elem, _ := p.ring.local(pid)
	ptr := elem.Swap(nil)
	procUnpin()

	if ptr != nil {
		return ptr
	}

	p.mutex.Lock()
	ptr = p.pool.Get()
	p.mutex.Unlock()
	return ptr
}

func (p *pool_internal) Put(ptr unsafe.Pointer) {
	if ptr != nil {
		// nil check
		// before procPin()
		_ = p.ring

		// put ptr.
		p.put(ptr)
	}
}

func (p *pool_internal) put(ptr unsafe.Pointer) {
	pid := procPin()
	elem, _ := p.ring.local(pid)
	ptr = elem.Swap(ptr)
	procUnpin()

	if ptr == nil {
		return
	}

	p.mutex.Lock()
	p.pool.put(ptr)
	p.mutex.Unlock()
}

func (p *pool_internal) GC() {
	p.ring.clear()
	p.mutex.Lock()
	p.pool.GC()
	p.mutex.Unlock()
}

func (p *pool_internal) Size() (sz int) {
	p.mutex.Lock()
	sz += p.pool.Size()
	p.mutex.Unlock()
	return
}

// locals_ring contains an atomically updated pointer to
// a ring buffer of pointer_elems, each accessed strictly
// by a single goroutine of known (and pinned) index.
//
// once a ring buffer is potentially in use, it is not
// possible to access any of the elems except individually
// within the guarantee of procPin(). even if you call clear(),
// you can never guarantee that another goroutine doesn't hold
// a pointer to the old ring buffer and is going to make a non
// atomic read / write to a particular pointer_elem.
type locals_ring struct{ p unsafe.Pointer }

// local returns an atomic_pointer from the fast-access ring buffer for given goroutine PID,
// returns pointer_elem{} and currently pinned goroutine PID (in case of repinning on alloc).
func (r *locals_ring) local(pid uint) (*pointer_elem, uint) {
	for {
		// Load current ring from ptr.
		ptr := atomic.LoadPointer(&r.p)
		if ptr != nil {

			// Check if pid within ring length.
			ring := *(*[]pointer_elem)(ptr)
			if pid < uint(len(ring)) {
				return &ring[pid], pid
			}
		}

		// Unpin before calling GOMAXPROCS,
		// which acquires a blocking mutex
		// lock on scheduler and may cause
		// goroutine to be rescheduled.
		procUnpin()

		// Allocate new ring buffer capable
		// of accomodating an index of 'pid'.
		maxprocs := runtime.GOMAXPROCS(0)
		ring := make([]pointer_elem, maxprocs)
		newptr := unsafe.Pointer(&ring)

		// Repin and get a (potentially)
		// different goroutine PID index.
		pid = procPin()

		// Attempt to replace
		// ring ptr with new.
		if pid < uint(len(ring)) &&
			atomic.CompareAndSwapPointer(&r.p,
				ptr,
				newptr) {
			return &ring[pid], pid
		}
	}
}

// clear will drop the current pointer to ring buffer.
func (r *locals_ring) clear() { atomic.StorePointer(&r.p, nil) }

// pointer_elem wraps an unsafe.Pointer to make
// swapping of a slice element nicer on the eyes.
type pointer_elem struct{ p unsafe.Pointer }

func (e *pointer_elem) Swap(p unsafe.Pointer) unsafe.Pointer {
	o := e.p
	e.p = p
	return o
}

// note is int in runtime, but should never be negative.
//
//go:linkname procPin runtime.procPin
func procPin() uint

//go:linkname procUnpin runtime.procUnpin
func procUnpin()
