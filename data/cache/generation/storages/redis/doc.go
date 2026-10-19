// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides a generation.Store shared through Redis, so a bump on
// one replica invalidates the generation-scoped caches of every replica.
//
// Bump runs an atomic increment. Current reads the key and keeps the value in
// a local cache for at most the max-staleness (one second by default), which
// is the explicit staleness bound: a Current call that starts more than
// max-staleness after a Bump returned sees the bump. The bumping replica
// sees its own bump at once. A missing key is seeded from the Redis server
// clock in microseconds, and a bump writes a value above both the stored one
// and every generation the bumping replica has seen, so a flushed or
// rolled-back Redis never makes old generations current
// again.
//
// A local TTL cache was chosen over pub/sub notification: its bound holds
// without a subscriber connection, reconnect handling or message delivery
// guarantees, and costs at most one GET per namespace per max-staleness.
//
// Failures never extend the bound: Current returns an error when Redis is
// unreachable and the local value is too old, and a namespace whose Bump
// failed reports an error on this replica until the increment is published.
// A stored generation below one the replica already confirmed (a failover
// restored an older value) fails the read with ErrRegressed and is healed
// by a bump above every known generation.
// Callers (data/cache) treat the error as "skip the cache".
//
// Example:
//
//	store := redis.New(client, redis.WithMaxStaleness(500*time.Millisecond))
//	c := cache.New(storage, cache.WithGeneration(store, "catalog"))
package redis
