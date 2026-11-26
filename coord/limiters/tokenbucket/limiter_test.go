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

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	"github.com/altessa-s/go-atlas/coord/limiters/storages"

	"github.com/altessa-s/go-atlas/coord/limiters/storages/memory"
	"github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"

	limiters "github.com/altessa-s/go-atlas/coord/limiters"
)

func validDefaults() tokenbucket.RateLimitSettings {
	return tokenbucket.RateLimitSettings{Limit: 10, Period: time.Minute}
}

// mustNew is a test helper that calls New and fails the test on error.
func mustNew(t *testing.T, defaults tokenbucket.RateLimitSettings, storage *memory.Provider, opts ...tokenbucket.Option) *tokenbucket.RuleLimiter {
	t.Helper()
	l, err := tokenbucket.New(defaults, storage, opts...)
	require.NoError(t, err)
	return l
}

func TestNew_Validation(t *testing.T) {
	t.Parallel()
	settings := &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}
	for _, tc := range []struct {
		name     string
		defaults tokenbucket.RateLimitSettings
		rules    []*tokenbucket.RateLimitRule
		wantErr  bool
	}{
		{"valid", validDefaults(), []*tokenbucket.RateLimitRule{{Target: "192.168.1.1", RateLimitSettings: settings}}, false},
		{"valid without rules", validDefaults(), nil, false},
		{"invalid default", tokenbucket.RateLimitSettings{Limit: 0, Period: time.Minute}, nil, true},
		{"invalid rule", validDefaults(), []*tokenbucket.RateLimitRule{{Target: "not-valid", RateLimitSettings: settings}}, true},
		{"nil rule", validDefaults(), []*tokenbucket.RateLimitRule{nil}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l, err := tokenbucket.New(tc.defaults, memory.New(), tokenbucket.WithRules(tc.rules...))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, l)
		})
	}
}

func TestLimit_IPBased_DefaultRule(t *testing.T) {
	cfg := validDefaults()
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
	defaults := tokenbucket.RateLimitSettings{Limit: 10, Period: time.Minute}
	rules := []*tokenbucket.RateLimitRule{
		{Target: "1.2.3.4", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 50, Period: time.Minute}},
	}
	l := mustNew(t, defaults, memory.New(),
		tokenbucket.WithRules(rules...),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "1.2.3.4"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(50), info.Limit)
}

func TestLimit_IPBased_CIDRRule(t *testing.T) {
	defaults := tokenbucket.RateLimitSettings{Limit: 10, Period: time.Minute}
	rules := []*tokenbucket.RateLimitRule{
		{Target: "10.0.0.0/8", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}},
	}
	l := mustNew(t, defaults, memory.New(),
		tokenbucket.WithRules(rules...),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "10.1.2.3"
		}),
	)

	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(100), info.Limit)
}

func TestLimit_IPBased_NoIP(t *testing.T) {
	l := mustNew(t, validDefaults(), memory.New())

	_, err := l.Limit(t.Context())
	require.ErrorIs(t, err, tokenbucket.ErrNoIPFoundOrInvalid)
}

func TestLimit_TokenBased(t *testing.T) {
	cfg := validDefaults()
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
	cfg := validDefaults()
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
	defaults := tokenbucket.RateLimitSettings{Limit: 1, Period: time.Minute}
	l := mustNew(t, defaults, memory.New(),
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
	cfg := validDefaults()
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
	cfg := validDefaults()
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
	cfg := validDefaults()
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
	l := mustNew(t, validDefaults(), memory.New(),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "not-an-ip"
		}),
	)

	_, err := l.Limit(t.Context())
	require.ErrorIs(t, err, tokenbucket.ErrNoIPFoundOrInvalid)
}

func TestRuleSorting(t *testing.T) {
	defaults := tokenbucket.RateLimitSettings{Limit: 1, Period: time.Minute}
	rules := []*tokenbucket.RateLimitRule{
		{Target: "10.0.0.0/8", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 100, Period: time.Minute}},
		{Target: "10.0.0.0/24", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 200, Period: time.Minute}},
		{Target: "10.0.0.1", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 300, Period: time.Minute}},
	}

	l := mustNew(t, defaults, memory.New(),
		tokenbucket.WithRules(rules...),
		tokenbucket.WithExtractClientIPAddress(func(_ context.Context) string {
			return "10.0.0.1"
		}),
	)

	// Exact IP (300) should take precedence over CIDR matches
	info, err := l.Limit(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(300), info.Limit)
}

func TestErrLimitExceededIdentity(t *testing.T) {
	require.Same(t, limiters.ErrLimitExceeded, tokenbucket.ErrLimitExceeded)
}

type failingRuleStorage struct {
	storages.Storage
	calls int
}

func (s *failingRuleStorage) Allow(context.Context, string, int64, time.Duration) (*storages.LimitInfo, error) {
	s.calls++
	return nil, errors.New("storage unavailable")
}

func TestLimit_UnlimitedIPSkipsStorage(t *testing.T) {
	t.Parallel()
	store := &failingRuleStorage{}
	l, err := tokenbucket.New(validDefaults(), store,
		tokenbucket.WithExtractClientIPAddress(func(context.Context) string { return "192.0.2.1" }),
		tokenbucket.WithRules(&tokenbucket.RateLimitRule{Target: "192.0.2.1", RateLimitSettings: tokenbucket.RateLimitUnlimited}),
	)
	require.NoError(t, err)
	got, err := l.Limit(t.Context())
	require.Zero(t, store.calls)
	require.NoError(t, err)
	require.Equal(t, tokenbucket.RateLimitUnlimited.LimitInfo(), got)
}

func TestLimit_InvalidClientSettingsSkipStorage(t *testing.T) {
	t.Parallel()
	store := &failingRuleStorage{}
	l, err := tokenbucket.New(validDefaults(), store,
		tokenbucket.WithExtractToken(func(context.Context) string { return "token" }),
		tokenbucket.WithClientService(tokenbucket.ClientServiceFunc(func(context.Context, string) (*tokenbucket.RateLimitSettings, error) {
			return &tokenbucket.RateLimitSettings{}, nil
		})),
	)
	require.NoError(t, err)
	_, err = l.Limit(t.Context())
	require.Zero(t, store.calls)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
}
