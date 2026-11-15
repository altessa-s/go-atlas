// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps a Bloom filter with the redis storage type to a Redis
// Bloom storage for factory/cache/probfilter.
//
// The probfilter factory builds only the in-memory storages; register this
// one for the redis type, on a filter or on the manager:
//
//	filter, err := probfilterfactory.NewFilter("users", cfg, defaults).
//		UseBloomStorage(probfilterconfig.StorageTypeRedis, bloomredisfactory.Builder(redisClient)).
//		Build(ctx)
//
// The storage keeps the filter under storage.redis.keysPrefix and fails
// Build on a server whose maxmemory-policy may evict the filter keys, unless
// the check is skipped.
package factory
