// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides Redis-based rate limit storage for distributed tokenbucket deployments.
// Uses sorted sets with atomic Lua scripts and sliding window algorithm for consistency.
//
// Example:
//
//	redisClient := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	storage := redis.New(redisClient, redis.WithKeyPrefix("myapp"))
//	defer storage.Close()
package redis
