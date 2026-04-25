// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package lru provides generic thread-safe and sharded LRU cache implementations.
// It supports standard and sharded modes, and provides singleflight-based GetOrCompute.
//
// Example:
//
//	cache, _ := lru.NewShardedCache[string, any](1000)
//	val, _ := cache.GetOrCompute(ctx, "key", func(ctx context.Context) (any, error) {
//	    return fetchData(), nil
//	})
package lru
