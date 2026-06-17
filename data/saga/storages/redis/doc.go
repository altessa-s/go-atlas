// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides a durable [saga.Store] backed by Redis.
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
// absent. FetchRecoverable is a ZRANGEBYSCORE over that set up to the current
// time. All writes keep the index consistent with the instance hash atomically.
//
// # Usage
//
//	store := redis.New(rdb, redis.WithKeyPrefix("saga:"))
//	orch := saga.New(store, def, saga.WithSagaTimeout(5*time.Minute))
package redis
