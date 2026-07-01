// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist"
	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/data/probfilter/bloom"

	bloommem "github.com/altessa-s/go-atlas/data/probfilter/bloom/storages/memory"
)

func TestFromChecker_BridgesDenylist(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	dl.Revoke("bad")
	auth := negcache.FromChecker(dl)

	got, err := auth.IsRevoked(t.Context(), "bad")
	require.NoError(t, err)
	require.True(t, got)

	got, err = auth.IsRevoked(t.Context(), "good")
	require.NoError(t, err)
	require.False(t, got)
}

// TestCache_EndToEndWithDenylist wires a real Bloom filter and an in-process
// denylist through FromChecker, exercising the full path: an unrevoked key is
// fast-pathed, a revoked key falls through and confirms revoked.
func TestCache_EndToEndWithDenylist(t *testing.T) {
	t.Parallel()
	dl := denylist.New()
	dl.Revoke("revoked-jti")

	c := negcache.New(bloom.New(bloommem.New()), negcache.FromChecker(dl))
	require.NoError(t, c.Add(t.Context(), "revoked-jti"))

	revoked, err := c.IsRevoked(t.Context(), "revoked-jti")
	require.NoError(t, err)
	require.True(t, revoked)

	fresh, err := c.IsRevoked(t.Context(), "never-seen")
	require.NoError(t, err)
	require.False(t, fresh)
}
