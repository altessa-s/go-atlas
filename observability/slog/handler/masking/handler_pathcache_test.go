// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPathCache_NeverGrowsForNilResults is the primary regression guard
// for the unbounded-cache fix. Before the fix, every miss (field path
// that did not match any configured mask) was stored as a nil entry,
// so an attacker driving dynamic group names — request IDs, tenant
// slugs, header names — could grow the cache without bound for the
// lifetime of the process.
//
// After the fix only positive matches are cached. We hammer the cache
// with 10_000 distinct non-matching paths and assert the size stays
// at zero.
func TestPathCache_NeverGrowsForNilResults(t *testing.T) {
	h := NewHandler(&captureHandler{store: &captureStore{}},
		WithField("Password", FullMask()),
	).(*Handler)

	for i := range 10_000 {
		// All distinct, none matching the "Password" rule.
		_ = h.getMaskForField("noise-"+strconv.Itoa(i), "noise-"+strconv.Itoa(i))
	}

	require.Zero(t, h.pathCacheCount.Load(),
		"nil mask results MUST NOT be cached — they are the exact growth vector the audit flagged")
}

// TestPathCache_PositiveHitsAreBoundedAndEpochSwap proves that even
// pathological hit-streams (more positive matches than cap allows) do
// NOT grow the cache past maxPathCacheEntries. The bound is enforced
// via an atomic epoch-swap: once the cap is reached the whole map is
// replaced with a fresh empty one. The test drives more inserts than
// the cap and asserts the count comes back under the cap after the
// swap.
func TestPathCache_PositiveHitsAreBoundedAndEpochSwap(t *testing.T) {
	// Pattern-based rule matches everything ⇒ every distinct path is a
	// positive hit and gets cached.
	h := NewHandler(&captureHandler{store: &captureStore{}},
		WithPattern(".*", FullMask()),
	).(*Handler)

	// Push past the cap. After the epoch swap the count must be small
	// (just the entries inserted after the swap), never above the cap.
	for i := range maxPathCacheEntries + 1000 {
		_ = h.getMaskForField("path-"+strconv.Itoa(i), "path-"+strconv.Itoa(i))
	}

	got := h.pathCacheCount.Load()
	require.LessOrEqual(t, got, int64(maxPathCacheEntries),
		"path cache must never exceed maxPathCacheEntries (got %d, cap %d)", got, maxPathCacheEntries)
}

// TestPathCache_CachedPositiveStillReturned guards the caching
// correctness side of the fix: a positive match must be cached on
// first call and returned from the cache on the second call (no
// re-resolution from the rules).
func TestPathCache_CachedPositiveStillReturned(t *testing.T) {
	h := NewHandler(&captureHandler{store: &captureStore{}},
		WithField("Password", FullMask()),
	).(*Handler)

	first := h.getMaskForField("Password", "Password")
	require.NotNil(t, first, "first call must resolve the mask")
	require.Equal(t, int64(1), h.pathCacheCount.Load(),
		"positive hit must be inserted into the cache")

	second := h.getMaskForField("Password", "Password")
	require.NotNil(t, second, "second call must return the cached mask")
	require.Equal(t, int64(1), h.pathCacheCount.Load(),
		"the same path must NOT double-count toward the cap")
}
