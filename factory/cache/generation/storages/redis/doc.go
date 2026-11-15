// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the redis block of the cache storage configuration to
// a shared Redis generation store for factory/cache, keyed under
// <keysPrefix>:generation:.
//
// The generation store builder builds only the in-process store; register
// this one for the redis type:
//
//	store, err := cachefactory.NewGenerationStore(&cfg.Cache).
//		UseStore(storageconfig.CacheStorageTypeRedis, redisgenfactory.Builder(redisClient)).
//		Build()
package factory
