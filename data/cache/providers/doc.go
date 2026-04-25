// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package providers defines the cache Provider interface and common errors.
// Implementations are in subpackages: freecache, redis, noop.
//
// Example:
//
//	var cache providers.Provider = redis.New(client)
//	cache.Save(ctx, "key", data, time.Hour)
package providers
