// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the contract suite for [scheduler.Storage]
// implementations. Every bundled backend runs it, and a custom backend should
// too: the scheduler's correctness rests on the atomic compare-and-swap writes,
// the run-ownership rules and the orderings it checks, which the in-memory
// backend gets for free and a real database must earn.
//
// [Run] executes, as parallel subtests on a fresh store each, every contract
// all bundled backends satisfy, so a new one cannot be left out of a single
// backend. [Identity], [Pagination] and [History] are run separately: the Redis
// and MongoDB backends do not satisfy them yet, while the memory and SQL
// backends do. Each contract is also exported on its own; it takes a fresh,
// isolated store (FinishRun and ReplaceTaskIf tolerate shared state; the others
// expect an empty store) and fails the test on any contract violation.
//
// # Usage
//
//	func TestStorageContract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, func(tb testing.TB) scheduler.Storage { return newStore(tb) })
//		for _, check := range []func(*testing.T, scheduler.Storage){
//			storagetest.Identity, storagetest.Pagination, storagetest.History,
//		} {
//			check(t, newStore(t))
//		}
//	}
package storagetest
