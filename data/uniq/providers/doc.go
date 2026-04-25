// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package providers defines the Provider interface for unique value storage.
// Implementations are in subpackages: nats, redis, noop.
//
// Example:
//
//	var store providers.Provider = redis.New(client)
//	store.Add(ctx, "unique-key")
package providers
