// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps a Cuckoo filter with the redis storage type to a Redis
// Cuckoo storage for factory/cache/probfilter.
//
// The probfilter factory builds only the in-memory storages; register this
// one for the redis type, on a filter or on the manager:
//
//	filter, err := probfilterfactory.NewFilter("sessions", cfg, defaults).
//		UseCuckooStorage(probfilterconfig.StorageTypeRedis, cuckooredisfactory.Builder(redisClient)).
//		Build(ctx)
//
// The storage keeps the filter under storage.redis.keysPrefix, maps
// capacityMultiplier to the RedisBloom EXPANSION (rounded up), and fails
// Build on a server whose maxmemory-policy may evict the filter keys, unless
// the check is skipped. Use [SentinelBuilder] for NewFailoverClusterClient.
// Both builders borrow their injected client.
package factory
