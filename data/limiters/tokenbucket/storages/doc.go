// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storages defines the Storage interface for token bucket rate limiting.
// Implementations are in subpackages: memory, redis, nats.
//
// Example:
//
//	var store storages.Storage = redis.New(client)
//	info, err := store.Allow(ctx, "user-123", 100, time.Minute)
package storages
