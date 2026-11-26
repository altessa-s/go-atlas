// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters"
	"github.com/altessa-s/go-atlas/coord/limiters/budget"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	redisstorage "github.com/altessa-s/go-atlas/coord/limiters/storages/redis"
	budgetfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/budget"
	factory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages/redis"
	tokenbucketfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/tokenbucket"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

func TestBuilder_BudgetEnforcesLimit(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)

	l, err := budgetfactory.New(&limiterconfig.Budget{
		Limit:   3,
		Period:  time.Hour,
		Storage: &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis, Redis: &storageconfig.RedisConfig{KeysPrefix: "budget:"}},
	}).UseStorage(limiterconfig.StorageTypeRedis, factory.Builder(client)).Build(t.Context())
	require.NoError(t, err)

	for i := range 3 {
		require.NoError(t, l.Allow(t.Context(), "tenant"), "call %d is within budget", i+1)
	}
	require.ErrorIs(t, l.Allow(t.Context(), "tenant"), budget.ErrBudgetExhausted)
	require.NotEmpty(t, mr.Keys())
	for _, k := range mr.Keys() {
		require.Contains(t, k, "budget:")
	}
}

func TestBuilder_TokenBucketEnforcesLimit(t *testing.T) {
	t.Parallel()
	client, mr := redistest.RedisClient(t)

	l, err := tokenbucketfactory.New(&limiterconfig.TokenBucket{
		IpCacheSize: 100,
		Storage:     &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis, Redis: &storageconfig.RedisConfig{KeysPrefix: "tb:"}},
		Rules: &limiterconfig.TokenBucketRules{
			Default: &limiterconfig.TokenBucketDefaultRule{Limit: 2, Period: time.Hour},
		},
	}).UseStorage(limiterconfig.StorageTypeRedis, factory.Builder(client)).Build(t.Context())
	require.NoError(t, err)

	ctx := limiters.ContextWithClientIP(t.Context(), "10.0.0.9")
	for i := range 2 {
		_, err := l.Limit(ctx)
		require.NoError(t, err, "request %d", i+1)
	}
	_, err = l.Limit(ctx)
	require.ErrorIs(t, err, limiters.ErrLimitExceeded)
	require.NotEmpty(t, mr.Keys())
	for _, k := range mr.Keys() {
		require.Contains(t, k, "tb:")
	}
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()

	_, err := factory.Builder(nil)(t.Context(),
		&limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis, Redis: &storageconfig.RedisConfig{}})
	require.ErrorIs(t, err, factory.ErrClientRequired)

	client, _ := redistest.RedisClient(t)
	_, err = factory.Builder(client)(t.Context(), &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis})
	require.ErrorIs(t, err, factory.ErrConfigRequired)
	require.ErrorIs(t, err, corefactory.ErrConfigRequired)
}

func TestBuilder_EmptyKeysPrefixUsesDefault(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		prefix string
	}{
		{name: "empty", prefix: ""},
		{name: "whitespace", prefix: "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, mr := redistest.RedisClient(t)

			l, err := budgetfactory.New(&limiterconfig.Budget{
				Limit:   2,
				Period:  time.Hour,
				Storage: &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis, Redis: &storageconfig.RedisConfig{KeysPrefix: tc.prefix}},
			}).UseStorage(limiterconfig.StorageTypeRedis, factory.Builder(client)).Build(t.Context())
			require.NoError(t, err)

			require.NoError(t, l.Allow(t.Context(), "tenant"))
			require.NotEmpty(t, mr.Keys())
			for _, k := range mr.Keys() {
				require.True(t, strings.HasPrefix(k, redisstorage.DefaultKeyPrefix), "key %q must use default prefix %q", k, redisstorage.DefaultKeyPrefix)
			}
		})
	}
}
