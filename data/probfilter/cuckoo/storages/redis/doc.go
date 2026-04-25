// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides a Redis-backed Cuckoo filter storage implementation
// using RedisBloom module CF.* commands.
//
// Requires Redis with RedisBloom module installed.
//
// Example:
//
//	storage := redis.New(rdb, "myfilter", redis.WithCapacity(100000))
//	defer storage.Close()
package redis
