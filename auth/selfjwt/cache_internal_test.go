// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	jwt "github.com/altessa-s/go-atlas/auth/jwt"
)

// stepClock is a Clock whose instant is advanced manually by tests.
type stepClock struct{ t time.Time }

func (c *stepClock) Now() time.Time { return c.t }

// put stores key unconditionally: the generation is read right before the
// insert, so no invalidation can fence it.
func put(c *keyCache, subject, kid string, key jwt.VerificationKey) {
	c.putIfGeneration(subject, kid, key, c.generation())
}

func TestKeyCacheMissThenHit(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	c := newKeyCache(time.Minute, 0, 100, clk)

	_, ok := c.get("s", "k")
	require.False(t, ok)

	put(c, "s", "k", jwt.VerificationKey{Algorithm: jwt.AlgEdDSA})
	got, ok := c.get("s", "k")
	require.True(t, ok)
	require.Equal(t, jwt.AlgEdDSA, got.Algorithm)
}

func TestKeyCacheExpiresLazily(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	c := newKeyCache(time.Minute, 0, 100, clk)

	put(c, "s", "k", jwt.VerificationKey{Algorithm: jwt.AlgEdDSA})
	clk.t = clk.t.Add(2 * time.Minute) // past the TTL

	_, ok := c.get("s", "k")
	require.False(t, ok)
}

func TestKeyCacheEnforcesMaxEntries(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	const max = 8
	c := newKeyCache(time.Hour, 0, max, clk) // long TTL: nothing expires during the loop

	// A churn of distinct kids must never grow the cache beyond the cap.
	for i := range max * 4 {
		put(c, "s", strconv.Itoa(i), jwt.VerificationKey{Algorithm: jwt.AlgEdDSA})
		require.LessOrEqual(t, c.size(), max)
	}
}

func TestKeyCachePutIfGenerationFencedByInvalidation(t *testing.T) {
	t.Parallel()
	vk := jwt.VerificationKey{Algorithm: jwt.AlgEdDSA}
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
			c := newKeyCache(time.Minute, 0, 100, &stepClock{t: time.Unix(0, 0).UTC()})
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

// Attacker-driven negative entries must not evict the verified keys that
// legitimate subjects rely on.
func TestKeyCacheNegativesDoNotEvictPositives(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	const max = 100
	c := newKeyCache(time.Hour, time.Hour, max, clk)

	const legit = 20
	for i := range legit {
		put(c, "legit", strconv.Itoa(i), jwt.VerificationKey{Algorithm: jwt.AlgEdDSA})
	}
	for i := range max * 3 {
		c.putMissIfGeneration("random-"+strconv.Itoa(i), "k", ErrSubjectUnknown, c.generation())
	}

	for i := range legit {
		_, ok := c.get("legit", strconv.Itoa(i))
		require.True(t, ok, "legitimate key %d was evicted by negative entries", i)
	}
}

func TestKeyCacheNegativeEntriesAreBounded(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	c := newKeyCache(time.Hour, time.Hour, 100, clk)

	for i := range 1000 {
		c.putMissIfGeneration("s"+strconv.Itoa(i), "k", ErrKeyRotated, c.generation())
	}
	require.LessOrEqual(t, c.size(), 100)
	require.ErrorIs(t, c.getMiss("s999", "k"), ErrKeyRotated, "the newest negative entry is kept")
}

func BenchmarkKeyCachePutAtCapacity(b *testing.B) {
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	for _, size := range []int{100, 10000} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			c := newKeyCache(time.Hour, time.Hour, size, clk)
			for i := range size {
				put(c, "s", strconv.Itoa(i), jwt.VerificationKey{Algorithm: jwt.AlgEdDSA})
			}
			vk := jwt.VerificationKey{Algorithm: jwt.AlgEdDSA}
			var i int
			for b.Loop() {
				i++
				put(c, "fresh", strconv.Itoa(i), vk)
			}
		})
	}
}
