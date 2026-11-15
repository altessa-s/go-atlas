// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides a fluent builder for creating cache providers
// from configuration.
//
// [StorageBuilder] uses a fluent API with deferred error accumulation:
// errors from any step are collected and returned at [StorageBuilder.Build] time.
//
//	provider, err := factory.New(cfg.Cache).
//	    UseLogger(logger).
//	    UseStorage(storageconfig.CacheStorageTypeRedis, redisfactory.Builder(redisClient)).
//	    Build()
//
// The builders build only the in-memory backends; the Redis ones register
// from factory/cache/storages/redis and
// factory/cache/generation/storages/redis, and a configured type
// without one fails at Build naming the package to add.
//
// [GenerationStoreBuilder] builds the matching cache generation store
// (in-process for memory, shared for redis) from the same configuration:
//
//	store, err := factory.NewGenerationStore(cfg.Cache).
//	    UseStore(storageconfig.CacheStorageTypeRedis, redisgenfactory.Builder(redisClient)).
//	    Build()
package factory
