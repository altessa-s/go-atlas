// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides Redis storage for idempotency keys with TTL support.
// Uses atomic SET NX operations for distributed idempotency checking.
//
// Example:
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	storage := redis.New(rdb, redis.WithTTL(time.Hour))
//	handler := idempotency.New(storage)
package redis
