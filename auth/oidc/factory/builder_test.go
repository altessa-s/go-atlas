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
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc"
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

// The failOpen config flag must map to oidc.WithRevocationFailOpen.
func TestProviderBuilder_BuildRevocationOptions_FailOpen(t *testing.T) {
	t.Parallel()

	cfg := memoryFilterRevocation()
	cfg.FailOpen = true
	b := New(&config.OIDC{Revocation: cfg}).UseRevocationStorage(stubStorage{})

	opts, err := b.buildRevocationOptions()
	require.NoError(t, err)
	require.Len(t, opts, 3)
}

// A YAML-configured URL source must reach the revocation list through the
// provider's shared HTTP client: NewProvider performs the initial sync, which
// fails (and, fail-closed, aborts construction) if the loader has no client.
func TestProviderBuilder_URLSourceSyncsThroughProviderClient(t *testing.T) {
	t.Parallel()

	var listHits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/revoked":
			listHits.Add(1)
			_, _ = w.Write([]byte("revoked-token\n"))
		case "/jwks":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"keys":[]}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"issuer":%q,"jwks_uri":%q,"id_token_signing_alg_values_supported":["RS256"]}`,
				srv.URL, srv.URL+"/jwks")
		}
	}))
	t.Cleanup(srv.Close)

	rev := memoryFilterRevocation()
	rev.Source = &config.OIDCRevocationSource{URL: srv.URL + "/revoked"}
	b := New(&config.OIDC{DiscoveryUrl: srv.URL, Revocation: rev}).UseRedisClient(offlineRedisClient(t))

	opts, err := b.buildProviderOptions(t.Context())
	require.NoError(t, err)
	// httptest serves plain HTTP on loopback.
	opts = append(opts, oidc.WithDiscoveryValidationMode(oidc.DiscoveryValidationModeDisabled))

	p, err := oidc.NewProvider(t.Context(), srv.URL, opts...)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	require.EqualValues(t, 1, listHits.Load(), "the initial sync must fetch the revocation list")
}

// Providers with different revocation domains must not share a filter (and
// so a Redis key); the same domain must map to the same filter across
// replicas.
func TestRevocationFilterName_BoundToRevocationDomain(t *testing.T) {
	t.Parallel()

	newCfg := func(discovery, itemType, url string) *config.OIDC {
		rev := memoryFilterRevocation()
		rev.ItemType = itemType
		rev.Source = &config.OIDCRevocationSource{URL: url}
		return &config.OIDC{DiscoveryUrl: discovery, Revocation: rev}
	}
	base := revocationFilterName(newCfg("https://a.example", "token", "https://a.example/revoked"))

	require.Equal(t, base, revocationFilterName(newCfg("https://a.example", "token", "https://a.example/revoked")))
	require.Regexp(t, `^oidc-revocation-[0-9a-f]{16}$`, base)
	for name, cfg := range map[string]*config.OIDC{
		"issuer":    newCfg("https://b.example", "token", "https://a.example/revoked"),
		"item type": newCfg("https://a.example", "jti", "https://a.example/revoked"),
		"source":    newCfg("https://a.example", "token", "https://a.example/other"),
	} {
		require.NotEqual(t, base, revocationFilterName(cfg), name)
	}
}

// The provider owns the revocation filter's rebuilds: per-filter rebuild
// settings that would add a second schedule are rejected, settings that
// disable them are accepted.
func TestProviderBuilder_BuildRevocationStorage_FilterRebuildSettings(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		cron    *string
		onStart *bool
		wantErr bool
	}{
		"rebuildCron":            {cron: new("@every 1h"), wantErr: true},
		"rebuildOnStart":         {onStart: new(true), wantErr: true},
		"disabled explicitly":    {cron: new(""), onStart: new(false)},
		"omitted (defaults off)": {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rev := memoryFilterRevocation()
			rev.Filter.Bloom.RebuildCron = tc.cron
			rev.Filter.Bloom.RebuildOnStart = tc.onStart
			b := New(&config.OIDC{Revocation: rev}).UseRedisClient(offlineRedisClient(t))

			_, err := b.buildRevocationStorage(rev)
			if tc.wantErr {
				require.ErrorIs(t, err, errFilterRebuildSettings)
				return
			}
			require.NoError(t, err)
		})
	}
}
