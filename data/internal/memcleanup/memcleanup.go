// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memcleanup

import (
	"sync"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// expiredPreallocDivisor sizes the phase-1 key buffer assuming roughly 10% of
// the entries are expired on a typical sweep.
const expiredPreallocDivisor = 10

// Sweep removes expired entries from entries using a two-phase strategy that
// keeps the write-lock window short:
//
//  1. Collect: under the read lock, evaluate expired for every entry and
//     collect the keys that report expired.
//  2. Delete: under the write lock, re-evaluate expired for each collected
//     key and delete only entries that are still present and still expired.
//
// The re-check in phase 2 is what makes the sweep safe against concurrent
// refreshes: an entry whose expiry was extended between the two phases (for
// example by a Complete/Store that re-armed its TTL under the write lock) is
// spared instead of being deleted with the stale phase-1 verdict.
//
// expired must be fast, must not touch mu, and must not mutate entries; it is
// invoked with mu held (read-locked in phase 1, write-locked in phase 2).
func Sweep[K comparable, V any](mu *sync.RWMutex, entries map[K]V, expired func(V) bool) {
	// Phase 1: identify expired keys under the read lock so concurrent
	// readers are not blocked while the whole map is scanned.
	mu.RLock()
	keys := make([]K, 0, len(entries)/expiredPreallocDivisor)
	for k, v := range entries {
		keys = coreslices.AppendIf(keys, expired(v), k)
	}
	mu.RUnlock()

	if len(keys) == 0 {
		return
	}

	// Phase 2: delete under the write lock, re-checking each entry in case
	// it was refreshed or replaced between the phases.
	mu.Lock()
	for _, k := range keys {
		if v, ok := entries[k]; ok && expired(v) {
			delete(entries, k)
		}
	}
	mu.Unlock()
}
