// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides in-memory rate limit storage for single-instance tokenbucket deployments.
// It applies a sliding window per key and is safe for concurrent use.
//
// # Capacity and cleanup
//
// [WithMaxBuckets] caps the number of tracked keys. A new key at the cap
// first tries to reclaim an expired window from the per-shard expiry indexes.
// When all windows are live, [EvictionReject] refuses the new key with
// [ErrCapacityExceeded]. [WithInsecureEvictLRU] allows eviction of live
// windows, so key churn can reset another key's quota before it expires.
// [Provider.RunCleanup] removes idle buckets whose windows have expired.
// Call it manually or schedule it with [WithScheduler] and [WithCleanupSchedule].
//
// # Multi-key admission
//
// [Provider.AllowAll] implements [storages.MultiStorage]: it admits a
// request against several keys or against none, locking their shards in
// ascending index order so overlapping calls cannot deadlock.
//
// # Sharding
//
// [WithShards] spreads keys over independently locked shards (default
// [DefaultShards]; values below 1 mean 1). Allow calls on keys in different
// shards then run in parallel, both for existing keys and for new keys while
// the provider is below its cap. The cap and LRU eviction stay exact and
// global, so a new key at the cap locks every shard. Use 8–16 shards when
// the provider usually runs below its cap on a many-core host, and keep 1
// when it is usually full with new keys arriving.
//
// Example:
//
//	storage := memory.New(
//		memory.WithShards(8),
//		memory.WithScheduler(scheduler),
//		memory.WithCleanupSchedule("@every 5m"),
//	)
//	defer storage.Close()
package memory
