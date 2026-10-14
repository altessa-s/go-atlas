// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterit_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/limiters"
	"github.com/altessa-s/go-atlas/data/limiters/budget"
	"github.com/altessa-s/go-atlas/data/limiters/storages"
	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"

	redislimiter "github.com/altessa-s/go-atlas/data/limiters/storages/redis"
)

// ruleLimiter builds a tokenbucket limiter reading the client IP and token
// from the context bridge, with a default of three requests, a /8 rule of
// five, and a client service granting "gold" two requests, "admin" no limit
// and everyone else the IP rules.
func ruleLimiter(t *testing.T, store storages.Storage) *tokenbucket.RuleLimiter {
	t.Helper()

	cfg := &tokenbucket.RateLimitConfig{
		Default: tokenbucket.RateLimitSettings{Limit: 3, Period: time.Minute},
		Rules: []*tokenbucket.RateLimitRule{
			{Target: "10.0.0.0/8", RateLimitSettings: &tokenbucket.RateLimitSettings{Limit: 5, Period: time.Minute}},
		},
	}
	clients := tokenbucket.ClientServiceFunc(func(_ context.Context, token string) (*tokenbucket.RateLimitSettings, error) {
		switch token {
		case "gold":
			return &tokenbucket.RateLimitSettings{Limit: 2, Period: time.Minute}, nil
		case "admin":
			return tokenbucket.RateLimitUnlimited, nil
		default:
			return tokenbucket.RateLimitSkip, nil
		}
	})
	l, err := tokenbucket.New(cfg, store,
		tokenbucket.WithExtractClientIPAddress(tokenbucket.ExtractClientIp),
		tokenbucket.WithExtractToken(tokenbucket.ExtractAuthToken),
		tokenbucket.WithClientService(clients),
	)
	require.NoError(t, err)
	return l
}

// TestTokenBucket_RulesOverSharedStorage drives the rule limiter on every
// backend from two replicas: the rule a client matches sets its limit, the
// replicas count the same client against one window, and the LimitInfo they
// return carries the storage's Remaining and Reset.
func TestTokenBucket_RulesOverSharedStorage(t *testing.T) {
	t.Parallel()

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			replica := b.open(t)
			a, c := ruleLimiter(t, replica()), ruleLimiter(t, replica())
			replicas := []*tokenbucket.RuleLimiter{a, c}
			ctx := t.Context()

			// Default rule: three requests, spread over both replicas.
			public := tokenbucket.ContextWithClientIP(ctx, "192.168.7.7")
			for i := range 3 {
				info, err := replicas[i%2].Limit(public)
				require.NoError(t, err, "request %d", i+1)
				require.Equal(t, int64(3), info.Limit)
				require.Equal(t, int64(2-i), info.Remaining, "request %d: replicas must count one window", i+1)
			}
			info, err := c.Limit(public)
			require.ErrorIs(t, err, limiters.ErrLimitExceeded)
			require.Equal(t, int64(3), info.Limit)
			require.Zero(t, info.Remaining)
			require.Greater(t, info.Reset, time.Now().Unix(), "a rejection reports when the window reopens")

			// CIDR rule: five requests for any address in 10/8.
			private := tokenbucket.ContextWithClientIP(ctx, "10.1.2.3")
			for i := range 5 {
				info, err := replicas[i%2].Limit(private)
				require.NoError(t, err, "request %d", i+1)
				require.Equal(t, int64(5), info.Limit)
			}
			_, err = a.Limit(private)
			require.ErrorIs(t, err, limiters.ErrLimitExceeded)

			// Token with its own settings: counted per token, not per IP.
			gold := tokenbucket.ContextWithAuthToken(tokenbucket.ContextWithClientIP(ctx, "192.168.7.7"), "gold")
			for range 2 {
				_, err := a.Limit(gold)
				require.NoError(t, err, "the token is not limited by its exhausted IP")
			}
			_, err = c.Limit(gold)
			require.ErrorIs(t, err, limiters.ErrLimitExceeded)

			// Unlimited token: never reaches the storage.
			admin := tokenbucket.ContextWithAuthToken(tokenbucket.ContextWithClientIP(ctx, "192.168.7.7"), "admin")
			for range 10 {
				info, err := c.Limit(admin)
				require.NoError(t, err)
				require.Equal(t, int64(math.MaxInt64), info.Remaining)
			}

			// Unknown token: falls back to its IP, which is exhausted.
			_, err = a.Limit(tokenbucket.ContextWithAuthToken(public, "bronze"))
			require.ErrorIs(t, err, limiters.ErrLimitExceeded)

			// No address at all.
			_, err = a.Limit(ctx)
			require.ErrorIs(t, err, tokenbucket.ErrNoIPFoundOrInvalid)
		})
	}
}

// TestBudget_SharedAcrossReplicas asserts the budget limiter on every
// backend: two replicas spend one budget, contention never overspends it, and
// it is available again a period later.
func TestBudget_SharedAcrossReplicas(t *testing.T) {
	t.Parallel()

	for _, b := range backends() {
		t.Run(b.name, func(t *testing.T) {
			t.Parallel()

			const limit = 12
			replica := b.open(t)
			ctx := t.Context()
			newPair := func(period time.Duration) (*budget.Limiter, *budget.Limiter) {
				first, err := budget.New(limit, period, replica())
				require.NoError(t, err)
				second, err := budget.New(limit, period, replica())
				require.NoError(t, err)
				return first, second
			}

			// A period far longer than the race, so every admission counts.
			first, second := newPair(time.Minute)
			replicas := []*budget.Limiter{first, second}
			res := hammer(16, 4, func(worker int) error {
				err := replicas[worker%2].Allow(ctx, "nightly-export")
				if errors.Is(err, budget.ErrBudgetExhausted) {
					return storages.ErrLimitExceeded // let the tally count it as a rejection
				}
				return err
			})
			require.LessOrEqual(t, res.admitted.Load(), int64(limit), "replicas overspent the budget")
			require.Empty(t, res.failures(), "calls failed without a decision")
			require.Equal(t, int64(limit), res.admitted.Load(), "a contended budget must still be spendable in full")
			require.ErrorIs(t, second.Allow(ctx, "nightly-export"), budget.ErrBudgetExhausted)

			// The shortest period: spent, then available again once it passes.
			short, other := newPair(budget.MinPeriod)
			for i := range limit {
				require.NoError(t, []*budget.Limiter{short, other}[i%2].Allow(ctx, "hourly-sync"))
			}
			require.ErrorIs(t, other.Allow(ctx, "hourly-sync"), budget.ErrBudgetExhausted)
			require.Eventually(t, func() bool {
				return short.Allow(ctx, "hourly-sync") == nil
			}, budget.MinPeriod+3*time.Second, 50*time.Millisecond, "the budget never came back after its period")
		})
	}
}

// TestBudget_StorageFailureIsNotExhaustion asserts fail-closed behavior on a
// real outage: a storage that cannot be reached is reported as an error, not
// as an exhausted budget a caller might treat as routine back-pressure.
func TestBudget_StorageFailureIsNotExhaustion(t *testing.T) {
	t.Parallel()

	client := redisClient(t)
	l, err := budget.New(5, time.Minute, redislimiter.New(client, redislimiter.WithKeyPrefix(namespace(t))))
	require.NoError(t, err)
	require.NoError(t, client.Close())

	err = l.Allow(t.Context(), "offline")
	require.Error(t, err)
	require.NotErrorIs(t, err, budget.ErrBudgetExhausted)
}
