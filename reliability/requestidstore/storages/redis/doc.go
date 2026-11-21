// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis stores AIP-155 request ID entries in Redis.
//
// # Usage
//
//	store := redis.New(client, redis.WithKeyPrefix("orders:requestid:"))
//	guard, err := requestid.New(store)
package redis
