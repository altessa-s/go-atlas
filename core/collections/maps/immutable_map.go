// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps

import (
	"hash/maphash"
	"iter"
	"math/bits"
)

const (
	groupSize = 8

	// allEmpty is a group word where every byte lane is 0x80 (empty sentinel).
	// 0x80 is chosen so that bit 7 distinguishes empty (1) from occupied (0),
	// and matchEmpty can isolate it with (word & ~(word<<1) & 0x80…).
	allEmpty = uint64(0x80808080_80808080)

	// loadNum/loadDen encode the target load factor 7/8 = 87.5%, matching the
	// Swiss-table convention. The table is sized to n*8/7 slots at construction.
	loadNum = 7
	loadDen = 8
)

// ImmutableMap is a read-only hash map that cannot be modified after
// construction. It is optimized for scenarios where data is built once and
// read many times: configuration tables, lookup dictionaries, static datasets.
//
// Advantages over a standard Go map:
//   - Immutability guarantee — safe to share across goroutines without
//     synchronization or defensive copies.
//   - Lower memory overhead — flat arrays at 87.5% load factor with no
//     per-bucket pointers, overflow chains, or growth metadata.
//   - Reduced GC pressure — three flat slices (ctrl, keys, vals) instead
//     of the runtime map's pointer-heavy internal structure. For long-lived
//     maps with millions of entries, this materially reduces GC scan time.
//   - Predictable allocation — exactly 3 heap allocations regardless of size.
//
// Lookup performance is comparable to the standard Go map on Go 1.24+ (which
// also uses Swiss tables with SIMD internally). ImmutableMap uses a pure-Go
// SWAR (SIMD Within A Register) fallback for group scanning.
type ImmutableMap[K comparable, V any] struct {
	ctrl   []uint64 // one uint64 per group of 8 control bytes
	keys   []K
	vals   []V
	seed   maphash.Seed
	len    int
	groups uint64
}

// NewImmutableMap builds an [ImmutableMap] from a standard Go map.
// The source map is not retained. Passing nil returns an empty map.
func NewImmutableMap[K comparable, V any](src map[K]V) *ImmutableMap[K, V] {
	n := len(src)
	if n == 0 {
		return &ImmutableMap[K, V]{}
	}

	m := allocImmutable[K, V](n)

	for k, v := range src {
		m.insert(k, v)
	}

	return m
}

// NewImmutableMapFromEntries builds an [ImmutableMap] from parallel key and
// value slices. It panics if len(keys) != len(values). Duplicate keys are
// resolved by last-write-wins.
func NewImmutableMapFromEntries[K comparable, V any](keys []K, values []V) *ImmutableMap[K, V] {
	if len(keys) != len(values) {
		panic("maps: NewImmutableMapFromEntries: keys and values length mismatch")
	}

	n := len(keys)
	if n == 0 {
		return &ImmutableMap[K, V]{}
	}

	m := allocImmutable[K, V](n)

	for i := range keys {
		m.insert(keys[i], values[i])
	}

	return m
}

// Get returns the value associated with key and true, or the zero value and
// false if the key is not present. Get is safe for concurrent use.
func (m *ImmutableMap[K, V]) Get(key K) (V, bool) {
	if m.len == 0 {
		var zero V
		return zero, false
	}

	h := maphash.Comparable(m.seed, key)
	h2 := byte(h & 0x7F)
	group := (h >> 7) % m.groups

	for range m.groups {
		word := m.ctrl[group]
		base := int(group) << 3

		mask := matchByte(word, h2)
		for mask != 0 {
			slot := base + (bits.TrailingZeros64(mask) >> 3)
			if m.keys[slot] == key {
				return m.vals[slot], true
			}
			mask &= mask - 1
		}

		if matchEmpty(word) != 0 {
			var zero V
			return zero, false
		}

		group++
		if group >= m.groups {
			group = 0
		}
	}

	var zero V
	return zero, false
}

// Contains reports whether key is present in the map.
func (m *ImmutableMap[K, V]) Contains(key K) bool {
	_, ok := m.Get(key)
	return ok
}

// Len returns the number of entries.
func (m *ImmutableMap[K, V]) Len() int {
	return m.len
}

// All returns an iterator over all key-value pairs. Iteration order is
// not guaranteed and depends on internal hash placement.
func (m *ImmutableMap[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for g, word := range m.ctrl {
			if word == allEmpty {
				continue
			}
			base := g << 3
			filled := ^word & 0x80808080_80808080
			for filled != 0 {
				slot := base + (bits.TrailingZeros64(filled) >> 3)
				if !yield(m.keys[slot], m.vals[slot]) {
					return
				}
				filled &= filled - 1
			}
		}
	}
}

// Keys returns an iterator over all keys.
func (m *ImmutableMap[K, V]) Keys() iter.Seq[K] {
	return func(yield func(K) bool) {
		for g, word := range m.ctrl {
			if word == allEmpty {
				continue
			}
			base := g << 3
			filled := ^word & 0x80808080_80808080
			for filled != 0 {
				slot := base + (bits.TrailingZeros64(filled) >> 3)
				if !yield(m.keys[slot]) {
					return
				}
				filled &= filled - 1
			}
		}
	}
}

// Values returns an iterator over all values.
func (m *ImmutableMap[K, V]) Values() iter.Seq[V] {
	return func(yield func(V) bool) {
		for g, word := range m.ctrl {
			if word == allEmpty {
				continue
			}
			base := g << 3
			filled := ^word & 0x80808080_80808080
			for filled != 0 {
				slot := base + (bits.TrailingZeros64(filled) >> 3)
				if !yield(m.vals[slot]) {
					return
				}
				filled &= filled - 1
			}
		}
	}
}

// allocImmutable creates an ImmutableMap with pre-sized arrays for n entries.
func allocImmutable[K comparable, V any](n int) *ImmutableMap[K, V] {
	numSlots := nextGroupMultiple(n*loadDen/loadNum + 1)
	groups := uint64(numSlots / groupSize)

	ctrl := make([]uint64, groups)
	for i := range ctrl {
		ctrl[i] = allEmpty
	}

	return &ImmutableMap[K, V]{
		ctrl:   ctrl,
		keys:   make([]K, numSlots),
		vals:   make([]V, numSlots),
		seed:   maphash.MakeSeed(),
		groups: groups,
	}
}

// insert places a key-value pair into the table. If the key already exists,
// the value is overwritten. Panics if the table has no empty slots (indicates
// a sizing bug in allocImmutable).
func (m *ImmutableMap[K, V]) insert(key K, value V) {
	h := maphash.Comparable(m.seed, key)
	h2 := byte(h & 0x7F)
	group := (h >> 7) % m.groups

	for range m.groups {
		word := m.ctrl[group]
		base := int(group) << 3

		mask := matchByte(word, h2)
		for mask != 0 {
			slot := base + (bits.TrailingZeros64(mask) >> 3)
			if m.keys[slot] == key {
				m.vals[slot] = value
				return
			}
			mask &= mask - 1
		}

		emptyMask := matchEmpty(word)
		if emptyMask != 0 {
			bit := uint(bits.TrailingZeros64(emptyMask))
			pos := bit >> 3
			slot := base + int(pos)
			shift := pos << 3
			m.ctrl[group] = (m.ctrl[group] &^ (0xFF << shift)) | (uint64(h2) << shift)
			m.keys[slot] = key
			m.vals[slot] = value
			m.len++
			return
		}

		group++
		if group >= m.groups {
			group = 0
		}
	}

	panic("maps: ImmutableMap insert: table full")
}

// matchByte returns a bitmask where bit 7 of each byte lane is set if that
// byte in word equals needle. Uses the SWAR (SIMD Within A Register) technique.
//
// Precondition: needle must have bit 7 clear (i.e. needle <= 0x7F). This is
// always satisfied because h2 = hash & 0x7F. Violating this precondition may
// produce false positives due to borrow propagation across byte lanes.
func matchByte(word uint64, needle byte) uint64 {
	broadcast := uint64(needle) * 0x01010101_01010101
	diff := word ^ broadcast
	return (diff - 0x01010101_01010101) & ^diff & 0x80808080_80808080
}

// matchEmpty returns a bitmask where bit 7 of each byte lane is set if that
// byte is the empty sentinel (0x80). A byte of 0x80 has bit 7 set and bit 6
// clear, so (word & ~(word<<1)) isolates exactly those lanes.
func matchEmpty(word uint64) uint64 {
	return word & ^(word << 1) & 0x80808080_80808080
}

// nextGroupMultiple returns the smallest multiple of groupSize >= n.
func nextGroupMultiple(n int) int {
	return (n + groupSize - 1) &^ (groupSize - 1)
}
