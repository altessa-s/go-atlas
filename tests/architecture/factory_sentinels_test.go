// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package architecture_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	auditconfig "github.com/altessa-s/go-atlas/config/audit"
	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	lockconfig "github.com/altessa-s/go-atlas/config/lock"
	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
	probfilterconfig "github.com/altessa-s/go-atlas/config/probfilter"
	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	vaultconfig "github.com/altessa-s/go-atlas/config/vault"
	auditfactory "github.com/altessa-s/go-atlas/factory/audit"
	auditchfactory "github.com/altessa-s/go-atlas/factory/audit/storages/clickhouse"
	auditmongofactory "github.com/altessa-s/go-atlas/factory/audit/storages/mongo"
	oidcfactory "github.com/altessa-s/go-atlas/factory/auth/oidc"
	cachefactory "github.com/altessa-s/go-atlas/factory/cache"
	cachegenredisfactory "github.com/altessa-s/go-atlas/factory/cache/generation/storages/redis"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
	bloomredisfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter/bloom/storages/redis"
	cuckooredisfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter/cuckoo/storages/redis"
	cacheredisfactory "github.com/altessa-s/go-atlas/factory/cache/storages/redis"
	dlockfactory "github.com/altessa-s/go-atlas/factory/coord/dlock"
	dlockmongofactory "github.com/altessa-s/go-atlas/factory/coord/dlock/providers/mongo"
	dlocknatsfactory "github.com/altessa-s/go-atlas/factory/coord/dlock/providers/nats"
	leadelectfactory "github.com/altessa-s/go-atlas/factory/coord/leadelect"
	budgetfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/budget"
	tokenbucketfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/tokenbucket"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	tracingfactory "github.com/altessa-s/go-atlas/factory/observability/tracing"
	vaultfactory "github.com/altessa-s/go-atlas/factory/security/vault"
)

func TestFactorySentinels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		call func(context.Context) error
		want []error
	}{
		{
			name: "tracing unregistered otlp",
			call: func(ctx context.Context) error {
				cfg := observabilityconfig.DefaultTracing()
				cfg.Enabled = true
				cfg.Type = observabilityconfig.TracingTypeOTLP
				_, err := tracingfactory.New(&cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrNotRegistered},
		},
		{
			name: "tracing unknown type",
			call: func(ctx context.Context) error {
				cfg := observabilityconfig.DefaultTracing()
				cfg.Enabled = true
				cfg.Type = "zipkin"
				_, err := tracingfactory.New(&cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrUnsupported},
		},
		{
			name: "tracing unknown sampler",
			call: func(ctx context.Context) error {
				cfg := observabilityconfig.DefaultTracing()
				cfg.Enabled = true
				cfg.Type = observabilityconfig.TracingTypeConsole
				cfg.Sampler = &observabilityconfig.TracingSampler{Type: "sometimes"}
				_, err := tracingfactory.New(&cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrUnsupported},
		},
		{
			name: "tracing nil adapter builder",
			call: func(ctx context.Context) error {
				cfg := observabilityconfig.DefaultTracing()
				cfg.Enabled = true
				_, err := tracingfactory.New(&cfg).UseAdapter("custom", nil).Build(ctx)
				return err
			},
			want: []error{coreerrs.ErrInvalidArgument},
		},
		{
			name: "oidc nil config",
			call: func(ctx context.Context) error {
				_, err := oidcfactory.New(nil).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrConfigRequired},
		},
		{
			name: "oidc nil bloom storage func",
			call: func(ctx context.Context) error {
				_, err := oidcfactory.New(nil).UseRevocationBloomStorage(probfilterconfig.StorageTypeRedis, nil).Build(ctx)
				return err
			},
			want: []error{coreerrs.ErrInvalidArgument},
		},
		{
			name: "oidc nil cuckoo storage func",
			call: func(ctx context.Context) error {
				_, err := oidcfactory.New(nil).UseRevocationCuckooStorage(probfilterconfig.StorageTypeRedis, nil).Build(ctx)
				return err
			},
			want: []error{coreerrs.ErrInvalidArgument},
		},
		{
			name: "vault unknown auth method",
			call: func(ctx context.Context) error {
				cfg := &vaultconfig.Config{
					Address: "https://vault.example.com",
					Auth:    &vaultconfig.Auth{Method: "kerberos"},
				}
				_, err := vaultfactory.New(cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrUnsupported},
		},
		{
			name: "audit unknown storage type",
			call: func(ctx context.Context) error {
				cfg := &auditconfig.Config{Enabled: true, Storage: auditconfig.Storage{Type: "unknown"}}
				_, err := auditfactory.New(cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrUnsupported},
		},
		{
			name: "audit sqldb missing db",
			call: func(ctx context.Context) error {
				cfg := &auditconfig.Config{Enabled: true, Storage: auditconfig.Storage{Type: auditconfig.StorageTypeSQL}}
				_, err := auditfactory.New(cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrDependencyRequired, auditfactory.ErrSQLDBRequired},
		},
		{
			name: "audit mongo missing db",
			call: func(ctx context.Context) error {
				cfg := &auditconfig.Config{Enabled: true, Storage: auditconfig.Storage{Type: auditconfig.StorageTypeMongo}}
				_, err := auditfactory.New(cfg).UseStorage(auditconfig.StorageTypeMongo, auditmongofactory.Builder(nil)).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrDependencyRequired, auditmongofactory.ErrDatabaseRequired},
		},
		{
			name: "audit clickhouse missing conn",
			call: func(ctx context.Context) error {
				cfg := &auditconfig.Config{Enabled: true, Storage: auditconfig.Storage{Type: auditconfig.StorageTypeClickHouse}}
				_, err := auditfactory.New(cfg).UseStorage(auditconfig.StorageTypeClickHouse, auditchfactory.Builder(nil)).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrDependencyRequired, auditchfactory.ErrConnRequired},
		},
		{
			name: "cache redis storage missing client",
			call: func(context.Context) error {
				cfg := &storageconfig.CacheStorageConfig{
					Type:  storageconfig.CacheStorageTypeRedis,
					Redis: &storageconfig.RedisConfig{},
				}
				_, err := cachefactory.New(cfg).UseStorage(storageconfig.CacheStorageTypeRedis, cacheredisfactory.Builder(nil)).Build()
				return err
			},
			want: []error{corefactory.ErrDependencyRequired, cacheredisfactory.ErrClientRequired},
		},
		{
			name: "cache nil storage func",
			call: func(context.Context) error {
				cfg := &storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeRedis}
				_, err := cachefactory.New(cfg).UseStorage(storageconfig.CacheStorageTypeRedis, nil).Build()
				return err
			},
			want: []error{coreerrs.ErrInvalidArgument},
		},
		{
			name: "cache redis generation store missing client",
			call: func(context.Context) error {
				cfg := &storageconfig.CacheStorageConfig{
					Type:  storageconfig.CacheStorageTypeRedis,
					Redis: &storageconfig.RedisConfig{},
				}
				_, err := cachefactory.NewGenerationStore(cfg).UseStore(storageconfig.CacheStorageTypeRedis, cachegenredisfactory.Builder(nil)).Build()
				return err
			},
			want: []error{corefactory.ErrDependencyRequired, cachegenredisfactory.ErrClientRequired},
		},
		{
			name: "cache nil generation store func",
			call: func(context.Context) error {
				cfg := &storageconfig.CacheStorageConfig{Type: storageconfig.CacheStorageTypeRedis}
				_, err := cachefactory.NewGenerationStore(cfg).UseStore(storageconfig.CacheStorageTypeRedis, nil).Build()
				return err
			},
			want: []error{coreerrs.ErrInvalidArgument},
		},
		{
			name: "probfilter bloom redis missing client",
			call: func(ctx context.Context) error {
				defaults := probfilterconfig.NewDefaults()
				cfg := &probfilterconfig.Filter{
					Type: probfilterconfig.TypeBloom,
					Bloom: &probfilterconfig.BloomConfig{
						ExpectedItems: 100,
						Storage:       &probfilterconfig.Storage{Type: probfilterconfig.StorageTypeRedis},
					},
				}
				_, err := probfilterfactory.NewFilter("bf", cfg, &defaults).
					UseBloomStorage(probfilterconfig.StorageTypeRedis, bloomredisfactory.Builder(nil)).
					Build(ctx)
				return err
			},
			want: []error{corefactory.ErrDependencyRequired, bloomredisfactory.ErrClientRequired},
		},
		{
			name: "probfilter cuckoo redis missing client",
			call: func(ctx context.Context) error {
				defaults := probfilterconfig.NewDefaults()
				cfg := &probfilterconfig.Filter{
					Type: probfilterconfig.TypeCuckoo,
					Cuckoo: &probfilterconfig.CuckooConfig{
						Capacity: 100,
						Storage:  &probfilterconfig.Storage{Type: probfilterconfig.StorageTypeRedis},
					},
				}
				_, err := probfilterfactory.NewFilter("cf", cfg, &defaults).
					UseCuckooStorage(probfilterconfig.StorageTypeRedis, cuckooredisfactory.Builder(nil)).
					Build(ctx)
				return err
			},
			want: []error{corefactory.ErrDependencyRequired, cuckooredisfactory.ErrClientRequired},
		},
		{
			name: "probfilter nil bloom storage func",
			call: func(ctx context.Context) error {
				defaults := probfilterconfig.NewDefaults()
				_, err := probfilterfactory.NewFilter("bf", nil, &defaults).
					UseBloomStorage(probfilterconfig.StorageTypeRedis, nil).
					Build(ctx)
				return err
			},
			want: []error{coreerrs.ErrInvalidArgument},
		},
		{
			name: "probfilter nil cuckoo storage func",
			call: func(ctx context.Context) error {
				defaults := probfilterconfig.NewDefaults()
				_, err := probfilterfactory.NewFilter("cf", nil, &defaults).
					UseCuckooStorage(probfilterconfig.StorageTypeRedis, nil).
					Build(ctx)
				return err
			},
			want: []error{coreerrs.ErrInvalidArgument},
		},
		{
			name: "dlock nats missing conn",
			call: func(ctx context.Context) error {
				cfg := &lockconfig.DistributionLock{
					Provider: lockconfig.DistributionLockProviderNATS,
					NATS:     &lockconfig.DistributionLockNATS{},
				}
				_, err := dlockfactory.New(cfg).UseProvider(lockconfig.DistributionLockProviderNATS, dlocknatsfactory.Builder(nil)).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrDependencyRequired, dlocknatsfactory.ErrClientRequired},
		},
		{
			name: "dlock mongo missing db",
			call: func(ctx context.Context) error {
				cfg := &lockconfig.DistributionLock{
					Provider: lockconfig.DistributionLockProviderMongo,
					Mongo:    &lockconfig.DistributionLockMongo{},
				}
				_, err := dlockfactory.New(cfg).UseProvider(lockconfig.DistributionLockProviderMongo, dlockmongofactory.Builder(nil)).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrDependencyRequired, dlockmongofactory.ErrClientRequired},
		},
		{
			name: "leadelect nil config",
			call: func(ctx context.Context) error {
				_, err := leadelectfactory.New(nil).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrConfigRequired},
		},
		{
			name: "leadelect empty node id",
			call: func(ctx context.Context) error {
				_, err := leadelectfactory.New(&lockconfig.LeaderElector{}).WithKey("election").Build(ctx)
				return err
			},
			want: []error{corefactory.ErrConfigRequired},
		},
		{
			name: "leadelect unregistered nats provider",
			call: func(ctx context.Context) error {
				_, err := leadelectfactory.New(&lockconfig.LeaderElector{}).WithNodeId("node-1").Build(ctx)
				return err
			},
			want: []error{corefactory.ErrNotRegistered},
		},
		{
			name: "leadelect unsupported provider",
			call: func(ctx context.Context) error {
				_, err := leadelectfactory.New(&lockconfig.LeaderElector{Provider: "etcd"}).WithNodeId("node-1").Build(ctx)
				return err
			},
			want: []error{corefactory.ErrUnsupported},
		},
		{
			name: "budget limiter unregistered redis storage",
			call: func(ctx context.Context) error {
				cfg := limiterconfig.DefaultBudget()
				cfg.Limit = 10
				cfg.Storage = &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis}
				_, err := budgetfactory.New(&cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrNotRegistered},
		},
		{
			name: "budget limiter unsupported nats storage",
			call: func(ctx context.Context) error {
				cfg := limiterconfig.DefaultBudget()
				cfg.Limit = 10
				cfg.Storage = &limiterconfig.StorageConfig{Type: "nats"}
				_, err := budgetfactory.New(&cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrUnsupported},
		},
		{
			name: "token-bucket limiter unregistered redis storage",
			call: func(ctx context.Context) error {
				cfg := limiterconfig.DefaultTokenBucket()
				cfg.Storage = &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis}
				_, err := tokenbucketfactory.New(&cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrNotRegistered},
		},
		{
			name: "token-bucket limiter unsupported nats storage",
			call: func(ctx context.Context) error {
				cfg := limiterconfig.DefaultTokenBucket()
				cfg.Storage = &limiterconfig.StorageConfig{Type: "nats"}
				_, err := tokenbucketfactory.New(&cfg).Build(ctx)
				return err
			},
			want: []error{corefactory.ErrUnsupported},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.call(t.Context())
			require.Error(t, err)
			for _, target := range tc.want {
				require.ErrorIs(t, err, target)
			}
		})
	}
}
