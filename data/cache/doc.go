// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package cache provides a unified caching interface with multiple backends and serialization support.
// Supports Redis, FreeCache (in-memory), LRU, and no-op providers with automatic fallback,
// singleflight deduplication, and configurable TTL.
//
// # Features
//
//   - Consistent API across different storage backends.
//   - Automatic fallback with singleflight deduplication to prevent cache stampede.
//   - Custom serialization (JSON, MessagePack, Protobuf).
//   - Distributed caching patterns with automatic key prefixing and TTL management.
//   - Factory pattern for declarative configuration.
//
// # Usage
//
//	provider := redis.New(redisClient, redis.WithPrefix("myapp:"))
//	c := cache.New(provider, cache.WithTtl(10*time.Minute))
//
//	// Save and Get
//	err := c.Save(ctx, "user:123", userData)
//	var user User
//	err = c.Get(ctx, "user:123", &user)
//
//	// Fallback with Singleflight
//	err = c.GetWithFallback(ctx, "key", &result, func() (any, time.Duration, error) {
//	    return db.Fetch(), 5*time.Minute, nil
//	})
//
// # Performance
//
//   - Singleflight deduplication: Prevents duplicate work for concurrent requests.
//   - FreeCache: Zero-GC in-memory cache using off-heap storage.
//   - Context timeout: 15s default to prevent hanging operations.
package cache
