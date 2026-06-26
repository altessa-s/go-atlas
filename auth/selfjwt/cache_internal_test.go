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

func TestKeyCacheMissThenHit(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	c := newKeyCache(time.Minute, 100, clk)

	_, ok := c.get("s", "k")
	require.False(t, ok)

	c.put("s", "k", VerificationKey{Algorithm: AlgEdDSA})
	got, ok := c.get("s", "k")
	require.True(t, ok)
	require.Equal(t, AlgEdDSA, got.Algorithm)
}

func TestKeyCacheExpiresLazily(t *testing.T) {
	t.Parallel()
	clk := &stepClock{t: time.Unix(0, 0).UTC()}
	c := newKeyCache(time.Minute, 100, clk)

	c.put("s", "k", VerificationKey{Algorithm: AlgEdDSA})
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
		c.put("s", strconv.Itoa(i), VerificationKey{Algorithm: AlgEdDSA})
		require.LessOrEqual(t, len(c.items), max)
	}
}
