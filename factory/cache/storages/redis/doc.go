// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the redis block of the cache storage configuration to
// a Redis cache storage for factory/cache.
//
// The cache factory builds only the in-memory storage; register this one for
// the redis type:
//
//	storage, err := cachefactory.New(&cfg.Cache).
//		UseStorage(storageconfig.CacheStorageTypeRedis, redisfactory.Builder(redisClient)).
//		Build()
package factory
