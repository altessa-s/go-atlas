// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters/budget"
	"github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	limitersctx "github.com/altessa-s/go-atlas/coord/limiters"
	limitnats "github.com/altessa-s/go-atlas/coord/limiters/storages/nats"
	budgetfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/budget"
	factory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages/nats"
	tokenbucketfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/tokenbucket"
)

func budgetLimiter(ctx context.Context, js jetstream.JetStream, limit int64, storage *storageconfig.BackendConfig,
	contention *limiterconfig.NATSContention,
) (*budget.Limiter, error) {
	return budgetfactory.New(&limiterconfig.Budget{Limit: limit, Period: time.Hour, Storage: storage, NATSContention: contention}).
		UseStorage(storageconfig.BackendTypeNATS, factory.Builder(js)).
		Build(ctx)
}

func tokenBucketLimiter(ctx context.Context, js jetstream.JetStream, limit int64, storage *storageconfig.BackendConfig,
	contention *limiterconfig.NATSContention,
) (*tokenbucket.RuleLimiter, error) {
	return tokenbucketfactory.New(&limiterconfig.TokenBucket{
		IpCacheSize:    100,
		Storage:        storage,
		NATSContention: contention,
		Rules: &limiterconfig.TokenBucketRules{
			Default: &limiterconfig.TokenBucketDefaultRule{Limit: limit, Period: time.Hour},
		},
	}).UseStorage(storageconfig.BackendTypeNATS, factory.Builder(js)).Build(ctx)
}

// limiters builds each limiter over the given nats storage block.
var limiters = map[string]func(ctx context.Context, js jetstream.JetStream, storage *storageconfig.BackendConfig) error{
	"budget": func(ctx context.Context, js jetstream.JetStream, storage *storageconfig.BackendConfig) error {
		_, err := budgetLimiter(ctx, js, 1000, storage, nil)
		return err
	},
	"tokenbucket": func(ctx context.Context, js jetstream.JetStream, storage *storageconfig.BackendConfig) error {
		_, err := tokenBucketLimiter(ctx, js, 1000, storage, nil)
		return err
	},
}

// TestBuilder_MigrateBucketTTL pins that storage.nats.migrateBucketTTL
// reaches the NATS storage: without it a bucket with a different key TTL is
// rejected and left alone, with it the bucket is migrated to the storage TTL.
func TestBuilder_MigrateBucketTTL(t *testing.T) {
	t.Parallel()

	for name, build := range limiters {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ns := natstest.StartNATSServer(t)
			_, js := natstest.ConnectJetStream(t, ns)

			bucket := "factory-" + name
			_, err := limitnats.New(t.Context(), js, limitnats.WithBucket(bucket), limitnats.WithMaxAge(time.Hour))
			require.NoError(t, err)

			storage := func(migrate bool) *storageconfig.BackendConfig {
				return &storageconfig.BackendConfig{
					Type: storageconfig.BackendTypeNATS,
					NATS: &storageconfig.NATSConfig{Bucket: bucket, Replicas: 1, MigrateBucketTTL: migrate},
				}
			}

			require.ErrorIs(t, build(t.Context(), js, storage(false)), limitnats.ErrBucketTTLMismatch)
			require.Equal(t, time.Hour, natstest.KVBucketTTL(t, js, bucket), "a rejected build changed the bucket's TTL")

			require.NoError(t, build(t.Context(), js, storage(true)))
			require.Equal(t, limitnats.DefaultMaxAge, natstest.KVBucketTTL(t, js, bucket), "migrateBucketTTL must update the bucket")
		})
	}
}
func TestBuilder_BudgetEnforcesLimit(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	l, err := budgetLimiter(t.Context(), js, 2,
		&storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS, NATS: &storageconfig.NATSConfig{Bucket: "budget-cov", Replicas: 1}},
		&limiterconfig.NATSContention{MaxCASAttempts: 8, MaxCASBackoff: time.Millisecond})
	require.NoError(t, err)

	for i := range 2 {
		require.NoError(t, l.Allow(t.Context(), "tenant"), "call %d is within budget", i+1)
	}
	require.ErrorIs(t, l.Allow(t.Context(), "tenant"), budget.ErrBudgetExhausted)
}

func TestBuilder_TokenBucketEnforcesLimit(t *testing.T) {
	t.Parallel()
	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)

	l, err := tokenBucketLimiter(t.Context(), js, 2,
		&storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS, NATS: &storageconfig.NATSConfig{Bucket: "tb-cov", Replicas: 1}},
		&limiterconfig.NATSContention{MaxCASAttempts: 8, MaxCASBackoff: time.Millisecond})
	require.NoError(t, err)

	ctx := limitersctx.ContextWithClientIP(t.Context(), "10.0.0.7")
	for i := range 2 {
		_, err := l.Limit(ctx)
		require.NoError(t, err, "request %d", i+1)
	}
	_, err = l.Limit(ctx)
	require.ErrorIs(t, err, limitersctx.ErrLimitExceeded)
}

func TestBuilder_Errors(t *testing.T) {
	t.Parallel()
	storage := &storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS, NATS: &storageconfig.NATSConfig{Bucket: "b", Replicas: 1}}

	_, err := factory.Builder(nil)(t.Context(), storage, nil)
	require.ErrorIs(t, err, factory.ErrClientRequired)

	ns := natstest.StartNATSServer(t)
	_, js := natstest.ConnectJetStream(t, ns)
	_, err = factory.Builder(js)(t.Context(), &storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS}, nil)
	require.ErrorIs(t, err, factory.ErrConfigRequired)
}
