// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// These tests are white-box (package factory) on purpose: the logic worth
// covering here is dependency resolution and config→option mapping inside
// buildRevocationOptions/buildRevocationStorage. Reaching them through the
// exported Build would require a live OIDC discovery endpoint, which tests
// nothing extra about the builder itself.
package factory

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

// fakeAuthoritative is an exact revocation store that answers from a fixed set
// and records whether the builder actually wired it in.
type fakeAuthoritative struct {
	revoked map[string]bool
	calls   int
}

func (a *fakeAuthoritative) IsRevoked(_ context.Context, item string) (bool, error) {
	a.calls++
	return a.revoked[item], nil
}

// offlineRedisClient returns a client that is never dialed: every test using it
// configures a memory-backed filter, and the builder only requires the
// dependency to be present.
func offlineRedisClient(t *testing.T) redis.UniversalClient {
	t.Helper()
	c := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func memoryFilterRevocation() *config.OIDCRevocation {
	storage := config.ProbabilisticFilterStorageTypeMemory
	return &config.OIDCRevocation{
		Enabled:  true,
		ItemType: "token",
		Filter: &config.ProbabilisticFilterConfig{
			Type: config.ProbabilisticFilterTypeBloom,
			Bloom: &config.ProbabilisticFilterBloomConfig{
				Storage:       &storage,
				ExpectedItems: 1000,
			},
		},
	}
}

func TestProviderBuilder_Build_NilConfig(t *testing.T) {
	t.Parallel()

	_, err := New(nil).Build(t.Context())
	require.Error(t, err)
	require.Contains(t, err.Error(), "configuration is required")
}

func TestProviderBuilder_BuildRevocationOptions_NotConfigured(t *testing.T) {
	t.Parallel()

	b := New(&config.OIDC{})
	opts, err := b.buildRevocationOptions()
	require.NoError(t, err)
	require.Nil(t, opts)
}

func TestProviderBuilder_BuildRevocationOptions_Disabled(t *testing.T) {
	t.Parallel()

	cfg := memoryFilterRevocation()
	cfg.Enabled = false

	b := New(&config.OIDC{Revocation: cfg})
	opts, err := b.buildRevocationOptions()
	require.NoError(t, err)
	require.Nil(t, opts)
}

// Revocation storage is built from config only when the redis dependency is
// present, even for a memory-backed filter.
func TestProviderBuilder_BuildRevocationStorage_RequiresRedisClient(t *testing.T) {
	t.Parallel()

	b := New(&config.OIDC{Revocation: memoryFilterRevocation()})
	_, err := b.buildRevocationOptions()
	require.Error(t, err)
	require.Contains(t, err.Error(), "redis client")
}

// A caller-supplied storage bypasses both filter construction and the redis
// dependency.
func TestProviderBuilder_BuildRevocationOptions_CustomStorageBypassesRedis(t *testing.T) {
	t.Parallel()

	b := New(&config.OIDC{Revocation: memoryFilterRevocation()}).
		UseRevocationStorage(stubStorage{})

	opts, err := b.buildRevocationOptions()
	require.NoError(t, err)
	require.Len(t, opts, 2)
}

// The confirmer passed via UseRevocationAuthoritative must reach the storage:
// an item present only in the filter is not revoked once the exact store
// denies it. Without this wiring a filter false positive would revoke a valid
// token.
func TestProviderBuilder_BuildRevocationStorage_ConfirmsFilterHits(t *testing.T) {
	t.Parallel()

	auth := &fakeAuthoritative{revoked: map[string]bool{}}
	b := New(&config.OIDC{Revocation: memoryFilterRevocation()}).
		UseRedisClient(offlineRedisClient(t)).
		UseRevocationAuthoritative(auth)

	storage, err := b.buildRevocationStorage(b.cfg.Revocation)
	require.NoError(t, err)
	require.NotNil(t, storage)

	ctx := t.Context()
	require.NoError(t, storage.MarkRevoked(ctx, "token", 0))

	revoked, err := storage.IsRevoked(ctx, "token")
	require.NoError(t, err)
	require.False(t, revoked, "authoritative store must override the filter hit")
	require.Equal(t, 1, auth.calls, "authoritative store must be consulted")
}

// Without a confirmer the factory stays in lossy mode: the filter hit stands.
func TestProviderBuilder_BuildRevocationStorage_LossyWithoutAuthoritative(t *testing.T) {
	t.Parallel()

	b := New(&config.OIDC{Revocation: memoryFilterRevocation()}).
		UseRedisClient(offlineRedisClient(t))

	storage, err := b.buildRevocationStorage(b.cfg.Revocation)
	require.NoError(t, err)

	ctx := t.Context()
	require.NoError(t, storage.MarkRevoked(ctx, "token", 0))

	revoked, err := storage.IsRevoked(ctx, "token")
	require.NoError(t, err)
	require.True(t, revoked)
}

// stubStorage is a caller-supplied RevocationStorage used to assert that a
// custom storage short-circuits filter construction.
type stubStorage struct{}

func (stubStorage) IsRevoked(context.Context, string) (bool, error) { return false, nil }
func (stubStorage) MarkRevoked(context.Context, string, time.Duration) error {
	return nil
}
func (stubStorage) Sync(context.Context) error { return nil }
