// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/altessa-s/go-atlas-core/collections/slices"

	corecontext "github.com/altessa-s/go-atlas-core/context"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	cuckoostorages "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages"
	cuckooredis "github.com/altessa-s/go-atlas/cache/probfilter/cuckoo/storages/redis"
	probfilterfactory "github.com/altessa-s/go-atlas/factory/cache/probfilter"
)

// ErrClientRequired is returned when the Redis Cuckoo storage is built
// without a client.
var ErrClientRequired = errors.New("probfilter: redis client is required for Cuckoo storage type redis")

// evictionCheckTimeout bounds the eviction-policy check of the storage.
const evictionCheckTimeout = 5 * time.Second

// Builder returns the Cuckoo storage function of the redis type, ready for
// probfilterfactory.FilterBuilder.UseCuckooStorage and
// probfilterfactory.ManagerBuilder.UseCuckooStorage. The storage keeps the
// filter named after spec.Name through client under storage.redis.keysPrefix,
// maps capacityMultiplier to the RedisBloom EXPANSION (rounded up), and first
// checks that the server cannot evict its keys (unless
// spec.SkipEvictionPolicyCheck).
func Builder(client redis.UniversalClient) probfilterfactory.CuckooStorageFunc {
	return builder(client, func(ctx context.Context) (bool, error) { return cuckooredis.CheckEvictionPolicy(ctx, client) })
}

// SentinelBuilder builds storage borrowing a client created by
// redis.NewFailoverClusterClient. Its eviction check discovers the current
// primary through Sentinel. A managed client's SentinelCluster supplies this provenance.
func SentinelBuilder(client *redis.ClusterClient) probfilterfactory.CuckooStorageFunc {
	if client == nil {
		return Builder(nil)
	}
	return builder(client, func(ctx context.Context) (bool, error) { return cuckooredis.CheckSentinelEvictionPolicy(ctx, client) })
}

func builder(client redis.UniversalClient, check func(context.Context) (bool, error)) probfilterfactory.CuckooStorageFunc {
	return func(ctx context.Context, spec probfilterfactory.CuckooStorageSpec) (cuckoostorages.Storage, error) {
		if client == nil {
			return nil, ErrClientRequired
		}
		if err := checkEvictionPolicy(ctx, check, spec.StorageSpec); err != nil {
			return nil, err
		}
		cfg := spec.Storage
		opts := []cuckooredis.Option{
			cuckooredis.WithCapacity(spec.Capacity),
		}
		opts = slices.AppendIf(opts, spec.CapacityMultiplier > 0, cuckooredis.WithExpansion(int64(math.Ceil(spec.CapacityMultiplier))))
		opts = slices.AppendIfFunc(opts, cfg != nil && cfg.Redis != nil && cfg.Redis.KeysPrefix != "", func() []cuckooredis.Option {
			return []cuckooredis.Option{cuckooredis.WithKeyPrefix(cfg.Redis.KeysPrefix)}
		})
		return cuckooredis.New(client, spec.Name, opts...), nil
	}
}

// checkEvictionPolicy fails when the Redis server may evict the filter's
// keys (see [cuckooredis.CheckEvictionPolicy]) and logs when the policy
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
