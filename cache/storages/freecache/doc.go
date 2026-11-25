// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package freecache provides an in-memory cache provider using FreeCache.
// It offers high-performance caching with zero GC overhead.
//
// Positive TTLs round up to whole seconds. [Storage.Save] returns
// [ErrTTLOutOfRange] for a TTL above [MaxTTL]. Zero and negative TTLs have no expiry.
//
// Example:
//
//	p := freecache.New(freecache.WithMaxSize(50 * 1024 * 1024))
//	p.Save(ctx, "key", data, time.Hour)
package freecache
