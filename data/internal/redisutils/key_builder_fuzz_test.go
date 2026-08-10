// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redisutils_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/internal/redisutils"
)

// FuzzBuiltKeysStayInsideTheirPrefix is the namespace-isolation oracle.
//
// Every Redis-backed component in the repo — cache, limiter, idempotency,
// cursor storage — namespaces its keys through this builder, and several run
// per tenant. A key that escaped its prefix would read or overwrite another
// namespace's entry: a rate limiter counting someone else's requests, or an
// idempotency record replaying a different caller's response.
//
// The property is stated over the result, not the inputs: whatever the prefix
// and key contain, a configured prefix must be exactly what the built key
// starts with, and the original key must be exactly what follows the separator.
func FuzzBuiltKeysStayInsideTheirPrefix(f *testing.F) {
	f.Add("cache", "user:1")
	f.Add("", "user:1")
	f.Add("cache:", "user:1")
	f.Add("cache", "")
	f.Add("", "")
	f.Add("a:b:c:", "d:e:f")
	f.Add(":", ":")
	f.Add("tenant-a", "tenant-b:secret")

	f.Fuzz(func(t *testing.T, prefix, key string) {
		kb := redisutils.NewKeyBuilder(prefix)
		built := kb.Build(key)

		if !kb.HasPrefix() {
			require.Equal(t, key, built, "an unprefixed builder must pass the key through")
			return
		}

		want := kb.Prefix() + redisutils.DefaultSeparator + key
		require.Equal(t, want, built,
			"the built key is not prefix+separator+key: prefix=%q key=%q", prefix, key)

		require.True(t, strings.HasPrefix(built, kb.Prefix()),
			"a key escaped its namespace: prefix=%q key=%q built=%q", prefix, key, built)
	})
}

// FuzzDistinctKeysStayDistinct pins that the builder is injective for a fixed
// prefix: two different keys never render to one.
//
// A collision is how two callers share a cache entry or an idempotency record.
// It is the failure mode a separator-based scheme invites — a key already
// containing the separator, or a prefix ending in one — and it is silent, since
// both callers get a plausible answer.
func FuzzDistinctKeysStayDistinct(f *testing.F) {
	f.Add("cache", "a", "b")
	f.Add("cache", "a:b", "a")
	f.Add("", "a", "a:")
	f.Add("p:", ":x", "x")

	f.Fuzz(func(t *testing.T, prefix, first, second string) {
		if first == second {
			t.Skip("identical keys are meant to collide")
		}

		kb := redisutils.NewKeyBuilder(prefix)
		require.NotEqual(t, kb.Build(first), kb.Build(second),
			"two distinct keys share one namespace slot under prefix %q", prefix)
	})
}

// FuzzPatternCoversEveryBuiltKey pins the relationship between Build and
// Pattern.
//
// Pattern is what a SCAN or a bulk invalidation is issued with, so a pattern
// that does not cover a key the same builder produced leaves that entry behind
// forever — a cache that cannot be flushed, or a limiter bucket that never
// resets.
func FuzzPatternCoversEveryBuiltKey(f *testing.F) {
	f.Add("cache", "user:1")
	f.Add("", "user:1")
	f.Add("cache:", "")

	f.Fuzz(func(t *testing.T, prefix, key string) {
		kb := redisutils.NewKeyBuilder(prefix)

		pattern := kb.Pattern()
		require.True(t, strings.HasSuffix(pattern, "*"),
			"a scan pattern that is not a glob matches only itself: %q", pattern)

		literal := strings.TrimSuffix(pattern, "*")
		require.True(t, strings.HasPrefix(kb.Build(key), literal),
			"the builder's own key falls outside its scan pattern: key=%q pattern=%q", kb.Build(key), pattern)
	})
}
