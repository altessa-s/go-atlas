// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package probfilter provides probabilistic data structures for optimizing existence checks.
// Supports Bloom and Cuckoo filters with multiple storage backends (memory, Redis).
//
// Bloom filters are space-efficient but do not support deletion. Use [RebuildableFilter]
// for periodic rebuilds when the underlying dataset changes.
//
// Cuckoo filters support deletion but have slightly higher memory overhead. Use
// [DeletableFilter] when individual key removal is required. They are rebuildable too.
//
// Rebuilds are atomic: lookups keep seeing the previous contents until the
// rebuilt contents replace them in one step, a failed or canceled rebuild
// leaves them unchanged, and values added during the rebuild are kept.
//
// A [Manager] created with [WithCollector] records lookup, add, and rebuild
// metrics for every registered [ObservableFilter].
//
// Example with Bloom filter:
//
//	storage := memory.New(memory.WithExpectedItems(100000))
//	filter := bloom.New(storage)
//	filter.Add(ctx, "user:123")
//	exists, _ := filter.MightExist(ctx, "user:123")
//
// Example with Manager:
//
//	mgr := probfilter.NewManager()
//	mgr.Register("users", userFilter)
//	filter, _ := mgr.Get("users")
//	filter.Add(ctx, "user:456")
package probfilter
