// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/denylist/negcache"
	"github.com/altessa-s/go-atlas/auth/denylist/negcache/factory"
	"github.com/altessa-s/go-atlas/config"
)

// fakeAuth is an exact Authoritative store backed by a set of revoked keys.
type fakeAuth struct {
	revoked map[string]struct{}
}

func newFakeAuth(revoked ...string) *fakeAuth {
	a := &fakeAuth{revoked: make(map[string]struct{}, len(revoked))}
	for _, k := range revoked {
		a.revoked[k] = struct{}{}
	}
	return a
}

func (a *fakeAuth) IsRevoked(_ context.Context, key string) (bool, error) {
	_, ok := a.revoked[key]
	return ok, nil
}

// memoryBloomConfig returns a minimal, valid in-memory Bloom filter config.
func memoryBloomConfig() *config.ProbabilisticFilterConfig {
	storage := config.ProbabilisticFilterStorageTypeMemory
	return &config.ProbabilisticFilterConfig{
		Type: config.ProbabilisticFilterTypeBloom,
		Bloom: &config.ProbabilisticFilterBloomConfig{
			Storage:       &storage,
			ExpectedItems: 1000,
		},
	}
}

func TestBuild_EndToEnd(t *testing.T) {
	t.Parallel()

	defaults := config.DefaultProbabilisticFilterDefaults()
	auth := newFakeAuth("revoked-jti")

	cache, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, auth).Build()
	require.NoError(t, err)
	require.NotNil(t, cache)

	// A fresh key the authoritative store does not know is fast-pathed as not
	// revoked (definite filter miss, no authoritative fall-through needed).
	got, err := cache.IsRevoked(t.Context(), "fresh")
	require.NoError(t, err)
	require.False(t, got)

	// Add a revoked key so lookups fall through, then confirm the authoritative
	// store resolves it as revoked end-to-end.
	require.NoError(t, cache.Add(t.Context(), "revoked-jti"))

	got, err = cache.IsRevoked(t.Context(), "revoked-jti")
	require.NoError(t, err)
	require.True(t, got)
}

func TestBuild_WithMetrics(t *testing.T) {
	t.Parallel()

	defaults := config.DefaultProbabilisticFilterDefaults()

	cache, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, newFakeAuth()).
		UseMetrics(nil, "").
		Build()
	require.NoError(t, err)
	require.NotNil(t, cache)

	got, err := cache.IsRevoked(t.Context(), "fresh")
	require.NoError(t, err)
	require.False(t, got)
}

func TestBuild_NilAuthoritative(t *testing.T) {
	t.Parallel()

	defaults := config.DefaultProbabilisticFilterDefaults()

	cache, err := factory.NewBuilder("denylist", memoryBloomConfig(), &defaults, nil).Build()
	require.Error(t, err)
	require.Nil(t, cache)
}

func TestBuild_NilFilterConfig(t *testing.T) {
	t.Parallel()

	defaults := config.DefaultProbabilisticFilterDefaults()

	cache, err := factory.NewBuilder("denylist", nil, &defaults, newFakeAuth()).Build()
	require.Error(t, err)
	require.Nil(t, cache)
}

// Compile-time assurance that fakeAuth satisfies the injected interface.
var _ negcache.Authoritative = (*fakeAuth)(nil)
