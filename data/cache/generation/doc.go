// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package generation defines the cache generation [Store]: a map from a
// namespace to a number that only grows. A cache that embeds the generation in
// its keys is invalidated as a whole by one Bump, and a store shared by
// several replicas (Redis) invalidates the caches of all of them.
//
// Backends live in subpackages: storages/memory (in-process) and
// storages/redis (shared, with a short local cache whose staleness is
// bounded). The data/cache package consumes a Store through
// cache.WithGeneration and cache.NewGenerations.
//
// Example:
//
//	store := redis.New(client, redis.WithMaxStaleness(time.Second))
//	gen, err := store.Current(ctx, "catalog")
//	_, err = store.Bump(ctx, "catalog") // after a write
package generation
