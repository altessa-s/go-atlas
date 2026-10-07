// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the contract suite for [saga.Storage]
// implementations. Every bundled backend runs it, and a custom backend should
// too: the orchestrator's correctness rests on insert-if-absent creation, the
// version compare-and-swap of Update and the recovery predicate of
// FetchRecoverable, which the in-memory backend gets for free and a real
// database must earn.
//
// [Run] executes every contract as parallel subtests on a fresh store each, so
// a new one cannot be left out of a single backend. Each contract is also
// exported on its own; it takes an empty, isolated store and fails the test on
// any contract violation.
//
// # Precision
//
// Durable backends may keep CreatedAt, UpdatedAt, Deadline and step times at
// one-second precision, so the fixtures use whole seconds for them. LeaseUntil
// must round-trip at full precision, as [saga.Instance] requires. Versions are
// opaque: a contract only checks that a successful Update changes the version
// and writes it back, not that it counts from zero in steps of one.
//
// # Usage
//
//	func TestStorageContract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, func(tb testing.TB) saga.Storage { return newStore(tb) })
//	}
package storagetest
