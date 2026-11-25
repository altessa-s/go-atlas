// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package lru provides an LRU-based cache storage implementing the storages.Storage interface.
//
// This provider wraps the core LRU cache implementation with TTL support for use
// as a cache backend. Capacities below 128 use a single LRU shard. Larger
// caches may evict from a full shard while others still have room.
//
// # Usage
//
//	provider, err := lru.New(1000) // 1000 items max
//	if err != nil {
//	    return err
//	}
//
//	err = provider.Save(ctx, "key", []byte("value"), time.Hour)
//	data, err := provider.Get(ctx, "key")
package lru
