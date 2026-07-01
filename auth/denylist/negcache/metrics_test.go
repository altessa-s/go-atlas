// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package negcache_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

const lookupsMetric = "test_auth_denylist_negcache_lookups_total"

func TestMetrics_RecordsLookupPaths(t *testing.T) {
	t.Parallel()
	tc := testhelpers.NewTestCollector()
	c := negcache.New(newFakeFilter(), newFakeAuth("revoked"), negcache.WithMetrics(negcache.NewMetrics(tc, "")))

	// fast_negative: filter miss, no authoritative call.
	got, err := c.IsRevoked(t.Context(), "fresh")
	require.NoError(t, err)
	require.False(t, got)

	// authoritative_hit: add to filter so the lookup falls through; store revoked.
	require.NoError(t, c.Add(t.Context(), "revoked"))
	got, err = c.IsRevoked(t.Context(), "revoked")
	require.NoError(t, err)
	require.True(t, got)

	// authoritative_miss: filter false positive, store reports not revoked.
	require.NoError(t, c.Add(t.Context(), "false-positive"))
	got, err = c.IsRevoked(t.Context(), "false-positive")
	require.NoError(t, err)
	require.False(t, got)

	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, lookupsMetric, "result", "fast_negative", "filter", "ok"))
	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, lookupsMetric, "result", "authoritative_hit", "filter", "ok"))
	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, lookupsMetric, "result", "authoritative_miss", "filter", "ok"))
}

func TestMetrics_RecordsFilterErrorState(t *testing.T) {
	t.Parallel()
	tc := testhelpers.NewTestCollector()
	filter := newFakeFilter()
	filter.mightErr = errors.New("filter down")
	c := negcache.New(filter, newFakeAuth("jti"), negcache.WithMetrics(negcache.NewMetrics(tc, "")))

	got, err := c.IsRevoked(t.Context(), "jti") // filter error → confirm via authoritative store.
	require.NoError(t, err)
	require.True(t, got)

	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, lookupsMetric, "result", "authoritative_hit", "filter", "error"))
}

func TestMetrics_NilIsNoOp(t *testing.T) {
	t.Parallel()
	// New without WithMetrics leaves the metrics seam nil; recording is a no-op.
	c := negcache.New(newFakeFilter(), newFakeAuth())
	got, err := c.IsRevoked(t.Context(), "x")
	require.NoError(t, err)
	require.False(t, got)
}
