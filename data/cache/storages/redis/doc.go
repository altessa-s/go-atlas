// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides a Redis-backed cache provider.
// Supports key prefixing for namespace isolation.
//
// Example:
//
//	p := redis.New(client, redis.WithPrefix("myapp"))
//	p.Save(ctx, "key", data, time.Hour)
package redis
