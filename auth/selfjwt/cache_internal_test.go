// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stepClock is a Clock whose instant is advanced manually by tests.
type stepClock struct{ t time.Time }

func (c *stepClock) Now() time.Time { return c.t }

// put stores key unconditionally: the generation is read right before the
// insert, so no invalidation can fence it.
func put(c *keyCache, subject, kid string, key VerificationKey) {
	c.putIfGeneration(subject, kid, key, c.generation())
}

func TestKeyCacheMissThenHit(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	c := newKeyCache(time.Minute, 100, clk)

	_, ok := c.get("s", "k")
	require.False(t, ok)

	put(c, "s", "k", VerificationKey{Algorithm: AlgEdDSA})
	got, ok := c.get("s", "k")
	require.True(t, ok)
	require.Equal(t, AlgEdDSA, got.Algorithm)
}

func TestKeyCacheExpiresLazily(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	c := newKeyCache(time.Minute, 100, clk)

	put(c, "s", "k", VerificationKey{Algorithm: AlgEdDSA})
	clk.t = clk.t.Add(2 * time.Minute) // past the TTL

	_, ok := c.get("s", "k")
	require.False(t, ok)
}

func TestKeyCacheEnforcesMaxEntries(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	const max = 8
	c := newKeyCache(time.Hour, max, clk) // long TTL: nothing expires during the loop

	// A churn of distinct kids must never grow the cache beyond the cap.
	for i := range max * 4 {
		put(c, "s", strconv.Itoa(i), VerificationKey{Algorithm: AlgEdDSA})
		c.mu.RLock()
		n := len(c.items)
		c.mu.RUnlock()
		require.LessOrEqual(t, n, max)
	}
}

func TestKeyCachePutIfGenerationFencedByInvalidation(t *testing.T) {
	t.Parallel()
	vk := VerificationKey{Algorithm: AlgEdDSA}
	tests := []struct {
		name       string
		invalidate func(c *keyCache)
		stored     bool
	}{
		{name: "NoInvalidation", invalidate: func(*keyCache) {}, stored: true},
		{name: "DeleteKey", invalidate: func(c *keyCache) { c.deleteKey("s", "k") }},
		{name: "DeleteSubject", invalidate: func(c *keyCache) { c.deleteSubject("s") }},
		// The generation is cache-wide: invalidating another subject also fences.
		{name: "DeleteOtherSubject", invalidate: func(c *keyCache) { c.deleteSubject("other") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newKeyCache(time.Minute, 100, &stepClock{t: time.Unix(0, 0).UTC()})
			gen := c.generation()
			tc.invalidate(c)
			c.putIfGeneration("s", "k", vk, gen)
			_, ok := c.get("s", "k")
			require.Equal(t, tc.stored, ok)

			// A lookup that starts after the invalidation caches normally.
			c.putIfGeneration("s", "k", vk, c.generation())
			_, ok = c.get("s", "k")
			require.True(t, ok)
		})
	}
}
