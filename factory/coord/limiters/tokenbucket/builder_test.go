// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	limitnats "github.com/altessa-s/go-atlas/coord/limiters/storages/nats"
	natsfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages/nats"
	factory "github.com/altessa-s/go-atlas/factory/coord/limiters/tokenbucket"
)

func validConfig() *limiterconfig.TokenBucket {
	return &limiterconfig.TokenBucket{
		IpCacheSize: 100,
		Storage: &storageconfig.BackendConfig{
			Type:   storageconfig.BackendTypeMemory,
			Memory: &storageconfig.MemoryConfig{},
		},
		Rules: &limiterconfig.TokenBucketRules{
			Default: &limiterconfig.TokenBucketDefaultRule{
				Limit:  1000,
				Period: time.Hour,
			},
		},
	}
}

// TestBuild_WithClientService_AppliesClientLimit verifies the factory's
// UseClientService option is wired end-to-end: the adapter is consulted on
// token-authenticated requests and the settings it returns override the
// default rule, exhausting after the configured number of requests.
func TestBuild_WithClientService_AppliesClientLimit(t *testing.T) {
	var calls int
	const clientLimit = 3
	cs := tokenbucket.ClientServiceFunc(func(_ context.Context, _ string) (*tokenbucket.RateLimitSettings, error) {
		calls++
		return &tokenbucket.RateLimitSettings{Limit: clientLimit, Period: time.Hour}, nil
	})

	l, err := factory.New(validConfig()).UseClientService(cs).Build(t.Context())
	require.NoError(t, err)

	ctx := limiters.ContextWithAuthToken(t.Context(), "t-123")
	ctx = limiters.ContextWithClientIP(ctx, "127.0.0.1")

	for i := range clientLimit {
		info, err := l.Limit(ctx)
		require.NoError(t, err, "request %d within client-specific limit should be allowed", i+1)
		require.Equal(t, int64(clientLimit), info.Limit, "LimitInfo must reflect the client-specific limit, not the default rule")
	}

	_, err = l.Limit(ctx)
	require.ErrorIs(t, err, limiters.ErrLimitExceeded, "request past client-specific limit must be rejected")
	require.Equal(t, clientLimit+1, calls, "ClientService.Settings must be consulted on every token-authenticated request")
}

// TestBuild_WithoutClientService_UsesDefaultRule verifies that when
// UseClientService is not called, token-authenticated
// requests fall through to the IP-based default rule (clientService field
// remains nil, guard in Build skips appending WithClientService).
func TestBuild_WithoutClientService_UsesDefaultRule(t *testing.T) {
	cfg := validConfig()
	cfg.Rules.Default.Limit = 7

	l, err := factory.New(cfg).Build(t.Context())
	require.NoError(t, err)

	ctx := limiters.ContextWithAuthToken(t.Context(), "some-token")
	ctx = limiters.ContextWithClientIP(ctx, "10.0.0.4")

	info, err := l.Limit(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(7), info.Limit, "without UseClientService, token-path requests must fall through to IP-based default rule")
}

func TestBuild_RejectsNATSWindowLongerThanRetention(t *testing.T) {
	t.Parallel()
	for _, location := range []string{"default", "target"} {
		t.Run(location, func(t *testing.T) {
			t.Parallel()
			ns := natstest.StartNATSServer(t)
			_, js := natstest.ConnectJetStream(t, ns)
			const bucket = "tokenbucket-period"
			const ttl = 48 * time.Hour
			_, err := limitnats.New(t.Context(), js, limitnats.WithBucket(bucket), limitnats.WithMaxAge(ttl))
			require.NoError(t, err)
			cfg := validConfig()
			cfg.Storage = &storageconfig.BackendConfig{
				Type: storageconfig.BackendTypeNATS,
				NATS: &storageconfig.NATSConfig{Bucket: bucket, MigrateBucketTTL: true},
			}
			if location == "default" {
				cfg.Rules.Default.Period = 25 * time.Hour
			} else {
				cfg.Rules.Targets = []limiterconfig.TokenBucketTargetRule{{Target: "192.0.2.1", Limit: 1, Period: 25 * time.Hour}}
			}
			_, err = factory.New(cfg).UseStorage(storageconfig.BackendTypeNATS, natsfactory.Builder(js)).Build(t.Context())
			require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
			require.Equal(t, ttl, natstest.KVBucketTTL(t, js, bucket))
		})
	}
}
