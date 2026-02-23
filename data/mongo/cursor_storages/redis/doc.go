// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides Redis cursor storage for MongoDB pagination.
// Enables distributed cursor sharing with automatic TTL-based expiration.
//
// Example:
//
//	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	storage := redisstorage.New(rdb, redisstorage.WithTTL(time.Hour))
//	result, _ := mongo.ListCursor(ctx, coll, mongo.WithListCursorStorage(storage))
package redis
