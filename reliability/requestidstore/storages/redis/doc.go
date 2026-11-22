// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis stores AIP-155 request ID entries in Redis.
//
// Each operation checks ExpiresAt against the caller's now. The physical TTL assumes that
// elapsed backend time can exceed elapsed caller time by less than [DefaultCleanupGrace]
// or the margin configured with [WithCleanupGrace].
//
// # Usage
//
//	store := redis.New(client, redis.WithKeyPrefix("orders:requestid:"))
//	guard, err := requestid.New(store)
package redis
