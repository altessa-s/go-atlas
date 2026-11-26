// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides a durable [saga.Storage] backed by Redis.
//
// Each saga instance is stored as a hash keyed by its ID, holding the
// serialized payload and a version field. Update is a version-checked write
// (a Lua compare-and-set on the version) that fails with errs.ErrVersionConflict
// when another coordinator has advanced the instance, so concurrent recovery
// cycles cannot double-advance it.
//
// # Recovery index
//
// Redis is not query-capable, so recoverable instances are tracked in a sorted
// set scored by recover-eligibility time: a timed-out RUNNING instance scores
// its deadline, a COMPENSATING instance scores 0, and any other instance is
// absent. FetchRecoverable reads the due ids page by page after a cursor
// (a script over ZRANGEBYSCORE ... LIMIT) and keeps only entries of the
// requested definition that are recoverable, so the transfer is bounded by the
// pages needed to fill the limit, not by the backlog. A terminal instance is
// listed in a second sorted set scored by its last update, which DeleteTerminal
// pages the same way. All writes keep both indexes consistent with the instance
// hash atomically.
//
// Scores are Unix milliseconds. With a TTL, every terminal write also drops the
// terminal-index entries older than the TTL, whose hashes have expired.
//
// # Cluster
//
// Every write is one script over the instance hash and both indexes, so on Redis
// Cluster the keys must share a hash slot. New rejects a cluster client whose
// key prefix has no hash tag; use a prefix such as "{saga}:".
//
// # Usage
//
//	store, err := redis.New(rdb, redis.WithKeyPrefix("saga:"))
//	orch := saga.New(store, def, saga.WithSagaTimeout(5*time.Minute))
package redis
