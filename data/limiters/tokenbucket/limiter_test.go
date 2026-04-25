// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tokenbucket_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters/storages/memory"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
)

func validConfig() *tokenbucket.RateLimitConfig {
	return &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 10, Period: time.Minute},
	}
}

// mustNew is a test helper that calls New and fails the test on error.
func mustNew(t *testing.T, config *tokenbucket.RateLimitConfig, storage *memory.Provider, opts ...tokenbucket.Option) *tokenbucket.RuleLimiter {
	t.Helper()
	l, err := tokenbucket.New(config, storage, opts...)
	require.NoError(t, err)
	return l
}

func TestNew_PanicsOnNilConfig(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("New(nil, ...) should panic")
		}
	}()
	tokenbucket.New(nil, memory.New()) //nolint:errcheck
}

func TestNew_ErrorOnInvalidConfig(t *testing.T) {
	_, err := tokenbucket.New(&tokenbucket.RateLimitConfig{}, memory.New())
	require.Error(t, err, "New(invalid, ...) should return error")
}

func TestNew_ValidConfig(t *testing.T) {
	l, err := tokenbucket.New(validConfig(), memory.New())
	require.NoError(t, err)
	require.NotNil(t, l, "New() returned nil")
}

func TestLimit_IPBased_DefaultRule(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.NotNil(t, info, "Limit() returned nil info")
	require.Equal(t, int64(10), info.Limit)
}

func TestLimit_IPBased_ExactIPRule(t *testing.T) {
	cfg := &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 10, Period: time.Minute},
		Rules: []*tokenbucket.RateLimitRule{
			{Target: "1.2.3.4", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 50, Period: time.Minute}},
		},
	}
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(50), info.Limit)
}

func TestLimit_IPBased_CIDRRule(t *testing.T) {
	cfg := &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 10, Period: time.Minute},
		Rules: []*tokenbucket.RateLimitRule{
			{Target: "10.0.0.0/8", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}},
		},
	}
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "10.1.2.3"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(100), info.Limit)
}

func TestLimit_IPBased_NoIP(t *testing.T) {
	l := mustNew(t, validConfig(), memory.New())

	_, err := l.Limit(t.Context())
	require.ErrorIs(t, err, tokenbucket.ErrNoIPFoundOrInvalid)
}

func TestLimit_TokenBased(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "test-token"
		}),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(_ context.Context, token string) (*tokenbucket.RateLimitSettings, error) {
				return &tokenbucket.RateLimitSettings{Limit: 200, Period: time.Hour}, nil
			},
		)),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(200), info.Limit)
}

func TestLimit_TokenBased_FallbackToIP(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "test-token"
		}),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(_ context.Context, token string) (*tokenbucket.RateLimitSettings, error) {
				return tokenbucket.RateLimitSkip, nil
			},
		)),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(10), info.Limit)
}

func TestLimit_Exceeded(t *testing.T) {
	cfg := &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 1, Period: time.Minute},
	}
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	// First request should succeed
	_, err := l.Limit(t.Context())
	require.NoError(t, err)

	// Second request should exceed
	_, err = l.Limit(t.Context())
	require.ErrorIs(t, err, tokenbucket.ErrLimitExceeded)
}

func TestLimit_TokenBased_Unlimited(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "vip-token"
		}),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(_ context.Context, token string) (*tokenbucket.RateLimitSettings, error) {
				return tokenbucket.RateLimitUnlimited, nil
			},
		)),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.True(t, tokenbucket.RateLimitUnlimited.IsUnlimited(), "sanity check: RateLimitUnlimited should be unlimited")
	require.Equal(t, info.Limit, info.Remaining)
}

func TestLimit_TokenBased_ClientServiceError_FallbackToIP(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "err-token"
		}),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(
			func(_ context.Context, token string) (*tokenbucket.RateLimitSettings, error) {
				return nil, errors.New("service unavailable")
			},
		)),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	// Should fall back to default IP-based limit
	require.Equal(t, int64(10), info.Limit)
}

func TestLimit_TokenBased_NoClientService(t *testing.T) {
	cfg := validConfig()
	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractToken(func(_ context.Context) string {
			return "some-token"
		}),
		// No WithClientService — handleRequest should fall through to IP
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(10), info.Limit)
}

func TestLimit_IPBased_InvalidIP(t *testing.T) {
	l := mustNew(t, validConfig(), memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "not-an-ip"
		}),
	)

	_, err := l.Limit(t.Context())
	require.ErrorIs(t, err, tokenbucket.ErrNoIPFoundOrInvalid)
}

func TestRuleSorting(t *testing.T) {
	cfg := &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 1, Period: time.Minute},
		Rules: []*tokenbucket.RateLimitRule{
			{Target: "10.0.0.0/8", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}},
			{Target: "10.0.0.0/24", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 200, Period: time.Minute}},
			{Target: "10.0.0.1", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 300, Period: time.Minute}},
		},
	}

	l := mustNew(t, cfg, memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "10.0.0.1"
		}),
	)

	// Exact IP (300) should take precedence over CIDR matches
	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(300), info.Limit)
}
