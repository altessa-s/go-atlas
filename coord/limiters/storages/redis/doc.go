// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides Redis-based rate limit storage for distributed tokenbucket deployments.
// Uses sorted sets with atomic Lua scripts and sliding window algorithm for consistency. Both scripts take window time from the Redis primary.
//
// [Provider.AllowAll] implements [storages.MultiStorage] with one script over
// every key. On a sharded or unknown client the keys of one call must
// share a hash tag, such as "{tenant-1}:global" and "{tenant-1}:host:a";
// otherwise the call fails with [ErrCrossSlot] before any network I/O. Untagged multi-key calls on
// raw single-node clients share one ROLE and INFO cluster check. The provider
// reuses a successful check for [DefaultTopologyVerificationWindow].
//
// Example:
//
//	redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	storage := redis.New(redisClient, redis.WithKeyPrefix("myapp"))
//	defer storage.Close()
package redis
