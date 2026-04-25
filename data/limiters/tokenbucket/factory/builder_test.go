// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket/factory"
)

func validConfig() *config.TokenBucketLimiter {
	return &config.TokenBucketLimiter{
		IpCacheSize: 100,
		Storage: &config.CacheStorageConfig{
			Type:   config.CacheStorageTypeMemory,
			Memory: &config.StorageMemoryConfig{},
		},
		Rules: &config.TokenBucketLimiterRules{
			Default: &config.TokenBucketLimiterDefaultRule{
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

	l, err := factory.New(validConfig()).UseClientService(cs).Build()
	require.NoError(t, err)

	ctx := tokenbucket.ContextWithAuthToken(context.Background(), "t-123")
	ctx = tokenbucket.ContextWithClientIP(ctx, "127.0.0.1")

	for i := 0; i < clientLimit; i++ {
		info, err := l.Limit(ctx)
		require.NoError(t, err, "request %d within client-specific limit should be allowed", i+1)
		require.Equal(t, int64(clientLimit), info.Limit, "LimitInfo must reflect the client-specific limit, not the default rule")
	}

	_, err = l.Limit(ctx)
	require.ErrorIs(t, err, tokenbucket.ErrLimitExceeded, "request past client-specific limit must be rejected")
	require.Equal(t, clientLimit+1, calls, "ClientService.Settings must be consulted on every token-authenticated request")
}

// TestBuild_WithoutClientService_UsesDefaultRule verifies backward
// compatibility: when UseClientService is not called, token-authenticated
// requests fall through to the IP-based default rule (clientService field
// remains nil, guard in Build skips appending WithClientService).
func TestBuild_WithoutClientService_UsesDefaultRule(t *testing.T) {
	cfg := validConfig()
	cfg.Rules.Default.Limit = 7

	l, err := factory.New(cfg).Build()
	require.NoError(t, err)

	ctx := tokenbucket.ContextWithAuthToken(context.Background(), "some-token")
	ctx = tokenbucket.ContextWithClientIP(ctx, "10.0.0.4")

	info, err := l.Limit(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(7), info.Limit, "without UseClientService, token-path requests must fall through to IP-based default rule")
}
