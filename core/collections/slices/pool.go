// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slices

import "sync"

const (
	// defaultSliceCapacity is the initial capacity used when creating pooled slices
	// if the caller does not specify a positive capacity in [NewPool].
	defaultSliceCapacity = 64

	// maxSlicePoolCapacity is the upper bound on slice capacity for returning a slice
	// to the pool. Slices whose capacity exceeds this threshold are discarded by
	// [Pool.Put] to prevent the pool from retaining excessively large allocations.
	// It is also the cap used by [EnsureCapacity] when rounding up to a power of two.
	maxSlicePoolCapacity = 1024
)

// Pool is a generic, concurrency-safe object pool for reusing []T slice buffers.
// It wraps [sync.Pool] and ensures that slices are reset to zero length before being
// handed out, so callers always receive an empty slice with pre-allocated capacity.
// Slices whose capacity exceeds [maxSlicePoolCapacity] are discarded by [Pool.Put]
// instead of being returned to the pool.
//
// Pool is safe for concurrent use by multiple goroutines.
type Pool[T any] struct {
	pool       sync.Pool
	defaultCap int
}

// NewPool creates a new [Pool] whose slices are initially allocated with defaultCap
// capacity. If defaultCap is zero or negative, [defaultSliceCapacity] (64) is used.
func NewPool[T any](defaultCap int) *Pool[T] {
	if defaultCap <= 0 {
		defaultCap = defaultSliceCapacity
	}
	return &Pool[T]{
		pool: sync.Pool{
			New: func() any {
				slice := make([]T, 0, defaultCap)
				return &slice
			},
		},
		defaultCap: defaultCap,
	}
}

// Get retrieves a slice pointer from the pool, resetting its length to zero while
// preserving the underlying capacity. If the pool is empty, a new slice with the
// default capacity is allocated. The caller must call [Pool.Put] when the slice is
// no longer needed to return it for reuse.
func (p *Pool[T]) Get() *[]T {
	slice, ok := p.pool.Get().(*[]T)
	if !ok {
		newSlice := make([]T, 0, p.defaultCap)
		slice = &newSlice
	}
	*slice = (*slice)[:0]
	return slice
}

// GetWithCapacity retrieves a slice from the pool, replacing it with a freshly
// allocated slice if the pooled slice's capacity is smaller than expectedCapacity
// (and expectedCapacity does not exceed [maxSlicePoolCapacity]). This avoids
// repeated grow-and-copy when the caller knows approximately how many elements will
// be appended. The caller must call [Pool.Put] when the slice is no longer needed.
func (p *Pool[T]) GetWithCapacity(expectedCapacity int) *[]T {
	slice := p.Get()

	if cap(*slice) < expectedCapacity && expectedCapacity <= maxSlicePoolCapacity {
		*slice = make([]T, 0, expectedCapacity)
	}

	return slice
}

// EnsureCapacity grows the slice pointed to by slicePtr so that its capacity is at
// least requiredCapacity, preserving existing elements. When growth is needed and
// requiredCapacity is at most [maxSlicePoolCapacity], the new capacity is rounded up
// to the next power of two for amortized efficiency. Returns true if a new backing
// array was allocated, false if the existing capacity was already sufficient.
func EnsureCapacity[T any](slicePtr *[]T, requiredCapacity int) bool {
	if cap(*slicePtr) >= requiredCapacity {
		return false
	}

	newCapacity := requiredCapacity
	if requiredCapacity <= maxSlicePoolCapacity {
		newCapacity = 1
		for newCapacity < requiredCapacity {
			newCapacity <<= 1
		}
		if newCapacity > maxSlicePoolCapacity {
			newCapacity = maxSlicePoolCapacity
		}
	}

	oldSlice := *slicePtr
	newSlice := make([]T, len(oldSlice), newCapacity)
	copy(newSlice, oldSlice)
	*slicePtr = newSlice

	return true
}

// Put returns a slice to the pool for reuse after clearing its elements and resetting
// its length to zero. If slice is nil or its capacity exceeds [maxSlicePoolCapacity],
// the slice is silently discarded to prevent the pool from retaining oversized
// allocations. After calling Put, the caller must not use slice again.
func (p *Pool[T]) Put(slice *[]T) {
	if slice == nil || cap(*slice) > maxSlicePoolCapacity {
		return
	}

	clear(*slice)
	*slice = (*slice)[:0]

	p.pool.Put(slice)
}
