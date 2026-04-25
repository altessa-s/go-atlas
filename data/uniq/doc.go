// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package uniq provides unique value management with multiple storage backends (Redis, NATS, no-op).
// Supports adding, checking existence, retrieving, and removing unique keys with optional values.
// Default TTL is 24 hours, JSON serialization is default but pluggable.
//
// Example:
//
//	u, _ := uniq.NewWithRedis(redisClient)
//	u.Add(ctx, "user:123")
//	exists, _ := u.Exist(ctx, "user:123")
//	u.AddWithValue(ctx, "session:abc", sessionData)
//	u.GetValue(ctx, "session:abc", &data)
package uniq
