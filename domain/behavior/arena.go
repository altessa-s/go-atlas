// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import (
	"reflect"
	"sync"
)

// arenaChunkLen is the minimum chunk capacity of pooled arenas. Pooling exists
// for hot paths, so chunks are sized to hold a typical resolved tree in one
// allocation; sync.Pool drops idle arenas under GC pressure either way.
const arenaChunkLen = 64

// arenaPool recycles arenas across walks. Acquired arenas batch their chunk
// allocations (chunkLen is set); a zero-value arena allocates exact-sized
// chunks, which keeps never-pooled walks allocation-equivalent to plain make.
var arenaPool = sync.Pool{New: func() any { return &arena{chunkLen: arenaChunkLen} }}

func acquireArena() *arena {
	return arenaPool.Get().(*arena) //nolint:errcheck // pool only contains *arena by design
}

// releaseArena resets a and returns it to the pool. Callers must not release
// an arena whose tree may still be referenced — after a translator panic the
// arena is abandoned to the GC instead.
//
// The reset's zeroing doubles as fail-fast enforcement of the [Translator]
// retention rule: a tree reference read after release holds zero
// reflect.Values, which panic on use instead of yielding stale data. Only a
// reference still held when the arena is reused can observe another walk's
// data.
func releaseArena(a *arena) {
	a.reset()
	arenaPool.Put(a)
}

// arena carves the walk's tree storage ([]Field, []Object, Collection boxes,
// map key slices) out of typed chunks, so a resolve pass costs a handful of
// chunk allocations instead of one per object — and none in steady state when
// the arena comes from arenaPool.
//
// Chunks never move: alloc sub-slices the current chunk and opens a new one
// when it is full, so slices handed out earlier in the walk stay valid. An
// append that outgrows its sub-slice capacity falls back to an ordinary heap
// reallocation — that only loses the batching, never corrupts.
type arena struct {
	// chunkLen is the minimum chunk capacity; zero makes every alloc
	// exact-sized (see arenaPool).
	chunkLen int

	fields  chunkList[Field]
	objects chunkList[Object]
	colls   chunkList[Collection]
	keys    chunkList[reflect.Value]
}

func (a *arena) allocFields(n int) []Field       { return a.fields.alloc(n, a.chunkLen) }
func (a *arena) allocObjects(n int) []Object     { return a.objects.alloc(n, a.chunkLen) }
func (a *arena) allocKeys(n int) []reflect.Value { return a.keys.alloc(n, a.chunkLen) }
func (a *arena) newCollection() *Collection      { return &a.colls.alloc(1, a.chunkLen)[0] }

// newObject boxes o in arena storage, replacing a heap-escaping &o.
func (a *arena) newObject(o Object) *Object {
	p := &a.objects.alloc(1, a.chunkLen)[0]
	*p = o
	return p
}

func (a *arena) reset() {
	a.fields.reset()
	a.objects.reset()
	a.colls.reset()
	a.keys.reset()
}

// chunkList is one typed chunk sequence of an arena: cur is carved by alloc,
// sealed chunks move to full so their sub-slices stay valid.
type chunkList[T any] struct {
	cur  []T
	full [][]T
	// sealed counts the elements in full, so grow can size the next chunk to
	// the whole walk so far (append-like doubling): a never-pooled arena
	// issues O(log n) chunks within ~2x of the exact allocation, and a pooled
	// one converges on a single right-sized chunk per type.
	sealed int
}

// alloc returns a zeroed slice of length n (capacity exactly n, so an
// overflowing append degrades to the heap instead of clobbering a neighbor).
// The fits-in-cur fast path is kept minimal so it inlines into the walk.
func (c *chunkList[T]) alloc(n, chunkLen int) []T {
	if cap(c.cur)-len(c.cur) < n {
		c.grow(n, chunkLen)
	}
	off := len(c.cur)
	c.cur = c.cur[:off+n]
	return c.cur[off : off+n : off+n]
}

// grow opens a new cur chunk, sealing the old one into full when it holds
// live data (an empty cur is just dropped to the GC).
func (c *chunkList[T]) grow(n, chunkLen int) {
	used := c.sealed + len(c.cur)
	if len(c.cur) > 0 {
		c.full = append(c.full, c.cur)
		c.sealed = used
	}
	c.cur = make([]T, 0, max(n, chunkLen, used))
}

// reset rewinds the list for reuse. Only cur survives in the pool, so it is
// the one chunk that must forget the previous walk's reflect.Values (and
// through them user objects); sealed chunks are unreferenced after the wipe
// below and the GC reclaims them together with whatever they point at.
func (c *chunkList[T]) reset() {
	clear(c.cur)
	c.cur = c.cur[:0]
	if c.sealed > 0 {
		clear(c.full)
		c.full = c.full[:0]
		c.sealed = 0
	}
}
