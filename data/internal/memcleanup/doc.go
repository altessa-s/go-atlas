// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memcleanup provides a generic two-phase TTL sweep for
// RWMutex-guarded in-memory maps. It is shared by the memory-backed storages
// under data/ (idempotency keys, MongoDB cursors) that periodically evict
// expired entries without blocking readers for the duration of a full scan.
//
// # Two-Phase Sweep
//
// [Sweep] first collects expired keys under the read lock, then deletes them
// under the write lock, re-checking each entry's expiry before deletion so an
// entry refreshed between the two phases survives:
//
//	now := time.Now()
//	memcleanup.Sweep(&s.mu, s.entries, func(e *entry) bool {
//	    return now.After(e.expiresAt)
//	})
//
// The predicate is invoked with the lock held and must be fast, must not
// touch the mutex, and must not mutate the map.
package memcleanup
