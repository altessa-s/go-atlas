// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps

import "sync"

const (
	// defaultMapCapacity is the initial capacity used when creating pooled maps
	// if the caller does not specify a positive capacity in [NewPool].
	defaultMapCapacity = 64

	// maxMapPoolCapacity is the upper bound on map length for returning a map to the
	// pool. Maps that have grown beyond this threshold are discarded by [Pool.Put]
	// to prevent the pool from retaining excessively large allocations.
	maxMapPoolCapacity = 1024
)

// Pool is a generic, concurrency-safe object pool for reusing map[K]V instances.
// It wraps [sync.Pool] and ensures that maps are cleared before being handed out,
// so callers always receive an empty map. Maps whose length exceeds
// [maxMapPoolCapacity] are discarded by [Pool.Put] instead of being returned to the pool.
//
// Pool is safe for concurrent use by multiple goroutines.
type Pool[K comparable, V any] struct {
	pool       sync.Pool
	defaultCap int
}

// NewPool creates a new [Pool] whose maps are initially allocated with defaultCap
// capacity. If defaultCap is zero or negative, [defaultMapCapacity] (64) is used.
func NewPool[K comparable, V any](defaultCap int) *Pool[K, V] {
	if defaultCap <= 0 {
		defaultCap = defaultMapCapacity
	}
	return &Pool[K, V]{
		pool: sync.Pool{
			New: func() any {
				m := make(map[K]V, defaultCap)
				return &m
			},
		},
		defaultCap: defaultCap,
	}
}

// Get retrieves a map pointer from the pool, clearing any residual entries so the
// caller receives an empty map. If the pool is empty, a new map with the default
// capacity is allocated. The caller must call [Pool.Put] when the map is no longer
// needed to return it for reuse.
func (p *Pool[K, V]) Get() *map[K]V {
	m, ok := p.pool.Get().(*map[K]V)
	if !ok {
		newMap := make(map[K]V, p.defaultCap)
		m = &newMap
	}

	clear(*m)
	return m
}

// GetWithCapacity retrieves a map from the pool, replacing it with a freshly allocated
// map if the pooled map's current size is smaller than expectedCapacity. This avoids
// early rehashing when the caller knows approximately how many entries will be inserted.
// The caller must call [Pool.Put] when the map is no longer needed.
func (p *Pool[K, V]) GetWithCapacity(expectedCapacity int) *map[K]V {
	m := p.Get()

	if expectedCapacity > len(*m) {
		*m = make(map[K]V, expectedCapacity)
	}

	return m
}

// Put returns a map to the pool for reuse after clearing its contents. If m is nil
// or its length exceeds [maxMapPoolCapacity], the map is silently discarded to prevent
// the pool from retaining oversized allocations. After calling Put, the caller must
// not use m again.
func (p *Pool[K, V]) Put(m *map[K]V) {
	if m == nil {
		return
	}

	if len(*m) > maxMapPoolCapacity {
		return
	}

	clear(*m)
	p.pool.Put(m)
}
