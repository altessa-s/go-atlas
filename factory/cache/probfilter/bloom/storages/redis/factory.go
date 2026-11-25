// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	bloomstorages "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages"
	bloomredis "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/redis"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
)

// ErrClientRequired is returned when the Redis Bloom storage is built without
// a client.
var ErrClientRequired = errors.New("probfilter: redis client is required for Bloom storage type redis")

// evictionCheckTimeout bounds the eviction-policy check of the storage.
const evictionCheckTimeout = 5 * time.Second

// Builder returns the Bloom storage function of the redis type, ready for
// probfilterfactory.FilterBuilder.UseBloomStorage and
// probfilterfactory.ManagerBuilder.UseBloomStorage. The storage keeps the
// filter named after spec.Name through client under storage.redis.keysPrefix,
// after checking that the server cannot evict its keys (unless
// spec.SkipEvictionPolicyCheck).
func Builder(client redis.UniversalClient) probfilterfactory.BloomStorageFunc {
	return builder(client, func(ctx context.Context) (bool, error) { return bloomredis.CheckEvictionPolicy(ctx, client) })
}

// SentinelBuilder builds storage borrowing a client created by
// redis.NewFailoverClusterClient. Its eviction check discovers the current
// primary through Sentinel. A managed client's SentinelCluster supplies this provenance.
func SentinelBuilder(client *redis.ClusterClient) probfilterfactory.BloomStorageFunc {
	if client == nil {
		return Builder(nil)
	}
	return builder(client, func(ctx context.Context) (bool, error) { return bloomredis.CheckSentinelEvictionPolicy(ctx, client) })
}

func builder(client redis.UniversalClient, check func(context.Context) (bool, error)) probfilterfactory.BloomStorageFunc {
	return func(ctx context.Context, spec probfilterfactory.BloomStorageSpec) (bloomstorages.Storage, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		if err := checkEvictionPolicy(ctx, check, spec.StorageSpec); err != nil {
			return nil, err
		}
		cfg := spec.Storage
		opts := []bloomredis.Option{
			bloomredis.WithExpectedItems(spec.ExpectedItems),
			bloomredis.WithFalsePositiveRate(spec.FalsePositiveRate),
		}
		opts = slices.AppendIfFunc(opts, cfg != nil && cfg.Redis != nil && cfg.Redis.KeysPrefix != "", func() []bloomredis.Option {
			return []bloomredis.Option{bloomredis.WithKeyPrefix(cfg.Redis.KeysPrefix)}
		})
		return bloomredis.New(client, spec.Name, opts...), nil
	}
}

// checkEvictionPolicy fails when the Redis server may evict the filter's
// keys (see [bloomredis.CheckEvictionPolicy]) and logs when the policy
// cannot be read.
func checkEvictionPolicy(ctx context.Context, check func(context.Context) (bool, error), spec probfilterfactory.StorageSpec) error {
	if spec.SkipEvictionPolicyCheck {
		return nil
	}
	ctx, cancel := corecontext.WithMaxTimeout(ctx, evictionCheckTimeout)
	defer cancel()
	verified, err := check(ctx)
	if err != nil {
		return coreerrs.Wrap(err, "redis storage of filter "+spec.Name)
	}
	if !verified && spec.Logger != nil {
		spec.Logger.Warn("probfilter: Redis maxmemory-policy not verified; an allkeys-* policy may evict filter keys",
			"filter", spec.Name)
	}
	return nil
}
