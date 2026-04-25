// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storages defines the Storage interface for idempotency key persistence.
// Implementations are in subpackages: memory, redis, nats.
//
// Example:
//
//	var store storages.Storage = redis.New(client)
//	store.Add(ctx, "request-123")
package storages
