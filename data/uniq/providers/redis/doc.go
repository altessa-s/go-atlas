// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides Redis storage for distributed unique values with TTL and prefix support.
// Supports standalone, cluster, and sentinel via UniversalClient. Clear uses Lua script for atomic batch deletion.
//
// Example:
//
//	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
//	provider, _ := redis.New(client, redis.WithPrefix("app:"), redis.WithTTL(1*time.Hour))
//	u := uniq.New(provider)
//	u.Add(ctx, "user:123")
package redis
