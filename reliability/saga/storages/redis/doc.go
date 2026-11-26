// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redis provides a durable [saga.Storage] backed by Redis.
//
// Each saga instance is stored as a hash holding the serialized payload and a version field. Update is a version-checked write
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
// Every write is one script over the instance hash and both indexes of one
// definition, so those keys carry the definition name as a hash tag
// ("<prefix>{d:<definition>}:i:<id>", "...:index:recoverable",
// "...:index:terminal") and share a slot on Redis Cluster, while different
// definitions spread across slots. The key prefix needs no hash tag of its own
// (one that has it places every key in a single slot). Storage calls that
// address an instance by ID alone (Get, Delete) resolve its definition through
// a plain key "<prefix>id:<id>", which also keeps IDs unique across
// definitions; a set "<prefix>definitions" lists the definitions for calls with
// an empty definition. Both live outside the one-slot instance scripts. Create
// claims the ID in the lookup, writes the instance, then re-checks the claim and
// rolls back when another definition won it, so IDs stay unique across
// definitions; Delete and retention remove the lookup before the instance. A
// lookup left behind by a crash reads as "not found" and is taken over by the
// next create. With a TTL the lookup of a terminal instance expires with it on a
// best-effort basis; a live instance's next write repairs an early expiry.
//
// An instance cannot change its definition: Update with another Definition
// fails with an error matching ErrInvalidArgument. A key prefix containing a
// brace is rejected by New, because it would take over the hash tag.
//
// # Upgrade
//
// Keys written by earlier versions ("<prefix>i:*", "<prefix>index:*") have no
// definition tag and are ignored: instances in flight under them are neither
// recovered nor listed. Finish or compensate in-flight sagas before upgrading,
// then delete the old keys.
//
// # Usage
//
//	store, err := redis.New(rdb, redis.WithKeyPrefix("saga:"))
//	orch := saga.New(store, def, saga.WithSagaTimeout(5*time.Minute))
package redis
