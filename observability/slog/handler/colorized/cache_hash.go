// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// cache_hash.go implements a lock-free hash-based color cache using atomic operations.

package colorized

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// FNV-1a hash constants.
const (
	fnv1aOffset = 2166136261
	fnv1aPrime  = 16777619
)

// colorCache is a specialized cache for colors using atomic operations.
//
// # Safety of unsafe.Pointer Usage
//
// The cache uses unsafe.Pointer to store *color.Color values. This pattern is safe because:
//
//  1. Type Consistency: The pointer always stores *color.Color values. All writes (putColor)
//     and reads (getColor) use the same type, ensuring type safety at the application level.
//
//  2. Atomic Operations: The atomic.Pointer[colorCacheEntry] ensures that pointer loads and
//     stores are atomic, preventing torn reads/writes in concurrent access.
//
//  3. Immutable After Creation: Color values are immutable once created and stored.
//     The cache never modifies a color after storing it, only reads it.
//
//  4. No Type Conversion: The unsafe.Pointer is used purely for storage to avoid generic
//     type constraints on color.Color. Callers cast back to *color.Color when retrieving.
//
//  5. GC Safety: The colorCacheEntry struct holds the unsafe.Pointer, keeping the
//     underlying color.Color object reachable by the garbage collector.
//
// This pattern is a standard Go idiom for storing typed pointers in generic containers
// without introducing additional type parameters or interface boxing overhead.
type colorCache struct {
	slots    [256]atomic.Pointer[colorCacheEntry] // Fixed-size array for common colors
	mu       sync.Mutex                           // Only for overflow map
	overflow map[colorKey]*colorCacheEntry        // Overflow for less common colors
}

// colorCacheEntry holds a cached color value.
// The color field uses unsafe.Pointer to store *color.Color without generic constraints.
type colorCacheEntry struct {
	key   colorKey
	color unsafe.Pointer // *color.Color - see colorCache documentation for safety invariants
}

// newLockFreeColorCache creates a new lock-free color cache
func newColorCache() *colorCache {
	return &colorCache{
		overflow: make(map[colorKey]*colorCacheEntry),
	}
}

// getColor retrieves a color from the cache by key.
// Returns the cached unsafe.Pointer (*color.Color) or nil if not found.
// The caller is responsible for casting the result to the appropriate type.
func (c *colorCache) getColor(key colorKey) unsafe.Pointer {
	// Hash to slot
	slot := hashColorKey(key) & 255 //nolint:mnd

	// Try to load from slot
	if entry := c.slots[slot].Load(); entry != nil && entry.key == key {
		return entry.color
	}

	// Check overflow map
	c.mu.Lock()
	if entry, ok := c.overflow[key]; ok {
		c.mu.Unlock()
		// Try to promote to slot
		c.slots[slot].CompareAndSwap(nil, entry)
		return entry.color
	}
	c.mu.Unlock()

	return nil
}

// putColor stores a color in the cache.
// The color parameter should be a *color.Color cast to unsafe.Pointer.
// The color value must be immutable after this call to ensure thread safety.
func (c *colorCache) putColor(key colorKey, color unsafe.Pointer) {
	entry := &colorCacheEntry{key: key, color: color}

	// Try to store in slot
	slot := hashColorKey(key) & 255 //nolint:mnd
	if c.slots[slot].CompareAndSwap(nil, entry) {
		return
	}

	// Store in overflow map
	c.mu.Lock()
	c.overflow[key] = entry
	c.mu.Unlock()
}

// hashColorKey computes a hash for color key using FNV-1a algorithm.
func hashColorKey(key colorKey) uint32 {
	hash := uint32(fnv1aOffset)
	for i := range key.len {
		hash ^= uint32(key.attrs[i]) // #nosec G115 -- truncation is intentional for hash
		hash *= fnv1aPrime
	}
	return hash
}
