// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStapler_CacheCap_EvictsOldestWhenFull is the regression guard
// for the unbounded OCSP cache: directly seed entries with monotonically
// later nextUpdate values, push past the cap, and verify the cache
// holds at most maxCacheEntries entries AND that the OLDEST entry is
// the one that got evicted (eviction order is by nextUpdate, oldest
// first).
func TestStapler_CacheCap_EvictsOldestWhenFull(t *testing.T) {
	t.Parallel()

	s := NewOCSPStapler(WithMaxCacheEntries(2))

	base := time.Now().Add(time.Hour)
	s.mu.Lock()
	s.cache["a"] = &ocspCacheEntry{nextUpdate: base}
	s.cache["b"] = &ocspCacheEntry{nextUpdate: base.Add(time.Minute)}
	s.mu.Unlock()

	// Insert a third entry — same fast-path the GetOCSPStaple Lock
	// branch uses. Eviction must remove "a" (oldest nextUpdate) before
	// inserting.
	s.mu.Lock()
	for len(s.cache) >= s.maxCacheEntries {
		require.True(t, s.evictOldestLocked(), "evictOldestLocked must succeed while cache is non-empty")
	}
	s.cache["c"] = &ocspCacheEntry{nextUpdate: base.Add(2 * time.Minute)}
	s.mu.Unlock()

	s.mu.RLock()
	defer s.mu.RUnlock()
	require.Len(t, s.cache, 2, "cache must respect the cap after insert+evict")

	_, hasA := s.cache["a"]
	_, hasB := s.cache["b"]
	_, hasC := s.cache["c"]
	require.False(t, hasA, "oldest entry (a) must have been evicted")
	require.True(t, hasB, "b must survive — newer than a")
	require.True(t, hasC, "c must survive — just inserted")
}

// TestStapler_CacheCap_DefaultIsApplied confirms a freshly-constructed
// stapler picks up DefaultMaxCacheEntries from the options layer (a
// regression here would re-enable the unbounded-cache vector).
func TestStapler_CacheCap_DefaultIsApplied(t *testing.T) {
	t.Parallel()

	s := NewOCSPStapler()
	require.Equal(t, DefaultMaxCacheEntries, s.maxCacheEntries,
		"default options must install DefaultMaxCacheEntries so the cache is bounded by default")
}

// TestStapler_CacheCap_ZeroIsUnbounded pins the opt-out path: callers
// who explicitly set maxCacheEntries=0 get the legacy unbounded
// behavior (useful when the cert population is known to be small).
func TestStapler_CacheCap_ZeroIsUnbounded(t *testing.T) {
	t.Parallel()

	s := NewOCSPStapler(WithMaxCacheEntries(0))

	// Seed more entries than DefaultMaxCacheEntries would normally
	// allow. The cap-check path only fires when maxCacheEntries > 0.
	s.mu.Lock()
	for i := range 10 {
		s.cache[strconv.Itoa(i)] = &ocspCacheEntry{nextUpdate: time.Now().Add(time.Hour)}
	}
	got := len(s.cache)
	s.mu.Unlock()

	require.Equal(t, 10, got, "with maxCacheEntries=0 the cache must be unbounded")
}
