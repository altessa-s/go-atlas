// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storages defines the Storage interface for rate limiting.
// Implementations are in subpackages: memory, redis, nats.
//
// Example:
//
//	var store storages.Storage = redis.New(client)
//	info, err := store.Allow(ctx, "user-123", 100, time.Minute)
//
// # Multi-key admission
//
// A request often has to pass several limits at once, such as a per-host
// and a global one. Two Allow calls cannot do that: when the second key
// refuses, the first has already taken a slot. [MultiStorage] is an
// optional interface whose AllowAll admits a request against every [Limit]
// or against none, and [AllowAll] calls it on any [Storage]:
//
//	infos, err := storages.AllowAll(ctx, store,
//		storages.Limit{Key: "{t1}:host:example.com", Limit: 10, Period: time.Second},
//		storages.Limit{Key: "{t1}:global", Limit: 100, Period: time.Second},
//	)
//	if keyErr, ok := errors.AsType[*storages.KeyError](err); ok && errors.Is(err, storages.ErrLimitExceeded) {
//		// limits[keyErr.Index] refused; no slot was taken on any key.
//	}
//
// [AllowAll] fails with [ErrMultiKeyUnsupported] for a storage without the
// interface and never falls back to sequential Allow calls. The memory and
// redis storages implement it; NATS KeyValue cannot update several keys
// atomically, so the nats storage does not. A call that names one key twice
// fails with [ErrDuplicateKey]. Third-party storages keep compiling: the
// [Storage] interface is unchanged, and [ValidateLimits] checks the
// arguments for an implementation.
package storages
