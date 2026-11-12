// GoToSocial
// Copyright (C) GoToSocial Authors admin@gotosocial.org
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package buffers

import (
	"math"
	"sync"
	"unsafe"

	"codeberg.org/gruf/go-byteutil"
	"codeberg.org/gruf/go-mempool"
)

var (
	// global map of base (unshareded) memory pools.
	pools = make(map[uint32]*mempool.UnsafePool, 8)

	// global map lock.
	mutex sync.Mutex
)

// MemoryPool is a memory pool of
// byte buffers, of a predefined size.
//
// This will be a shard of a shared global
// instance, itself a unique pointer but the
// underlying shard pointing back to the same
// underlying pool callers of buffers.Pool($sz)
// will all have access to.
type MemoryPool struct {
	p mempool.UnsafePoolShard
	s uint32
}

// Pool returns a shared MemoryPool instance
// of requested size (rounded to nearest 2^n).
//
// NOTE: this acquires a lock on a global mutex
// instance and should generally only be called
// on package init to store a global reference.
func Pool(sz uint32) *MemoryPool {

	// Calculate 2^n rounded
	// size of memory pool.
	n := math.Log2(float64(sz))
	n = math.Round(n)
	n = math.Exp2(max(8, n))
	sz = uint32(n)

	// Get lock.
	mutex.Lock()

	// Check for existing
	// memory pool of size.
	p := pools[sz]

	if p == nil {
		// Allocate new memory pool.
		p = new(mempool.UnsafePool)

		// Place in map.
		pools[sz] = p
	}

	// Done locking.
	mutex.Unlock()

	return &MemoryPool{
		p: p.Shard(),
		s: sz,
	}
}

// Get returns a byteutil.Buffer{} instance from pool.
func (p *MemoryPool) Get() *byteutil.Buffer {
	buf := (*byteutil.Buffer)(p.p.Get())
	if buf == nil {
		buf = new(byteutil.Buffer)
		buf.B = make([]byte, p.s)
	} else {
		clear(buf.B[0:cap(buf.B)])
	}
	buf.B = buf.B[:0]
	return buf
}

// Put replaces byteutil.Buffer{} instance in pool.
func (p *MemoryPool) Put(buf *byteutil.Buffer) {
	if buf == nil {
		return
	}
	if cap(buf.B) < int(p.s) || cap(buf.B) > 2*int(p.s) {
		return // drop buffers outside size range
	}
	p.p.Put(unsafe.Pointer(buf))
}

// buf1k is a pre-prepared memory pool
// of 1k kibibyte size byte buffers.
//
// this is a common-size so we provide
// an instance of it out-of-the-box.
var buf1k = Pool(1024)

// Get returns a byteutil.Buffer{} instance from the 1k
// pool, it will have a capacity of at-least 1024 bytes.
//
// NOTE: this is useful for the semi-frequent scratch
// buffer, but if you are making heavy usage of this
// function you should probably get your own package
// local pool instance with Pools($size).
func Get1k() *byteutil.Buffer {
	return buf1k.Get()
}

// Put replaces byteutil.Buffer{} instance in the 1k
// pool, it will be dropped if capacity < 1024 || > 2048.
//
// NOTE: this is useful for the semi-frequent scratch
// buffer, but if you are making heavy usage of this
// function you should probably get your own package
// local pool instance with Pools($size).
func Put1k(buf *byteutil.Buffer) {
	buf1k.Put(buf)
}
