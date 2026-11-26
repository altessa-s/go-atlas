// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating rate limiters
// from configuration.
//
// [TokenBucketLimiterBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [TokenBucketLimiterBuilder.Build] time.
//
//	limiter, err := factory.New(cfg.TokenBucketLimiter).
//	    UseLogger(logger).
//	    UseStorage(storageconfig.BackendTypeRedis, redisfactory.Builder(redisClient)).
//	    Build(ctx)
//
// The builder builds only the in-memory storage; the Redis and NATS ones
// register from factory/coord/limiters/storages/redis and
// factory/coord/limiters/storages/nats, and a configured type without one
// fails at Build naming the package to add. InsecureEvictLRU opts memory storage
// into live-window eviction.
//
// [TokenBucketLimiterBuilder.UseStorage] accepts custom backend names. For standalone schema
// validation, pass [TokenBucketLimiterBuilder.RegisteredStorageTypes] results to SetRegisteredTypes.
package factory
