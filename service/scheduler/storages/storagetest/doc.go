// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the contract suite for [scheduler.Storage]
// implementations. Every bundled backend runs it, and a custom backend should
// too: the scheduler's correctness rests on the atomic compare-and-swap writes,
// the run-ownership rules and the orderings it checks, which the in-memory
// backend gets for free and a real database must earn.
//
// [Run] executes every contract as parallel subtests on a fresh store each, so
// a new one cannot be left out of a single backend; every bundled backend
// satisfies all of them. Each contract is also exported on its own; it takes a fresh,
// isolated store (FinishRun and ReplaceTaskIf tolerate shared state; the others
// expect an empty store) and fails the test on any contract violation.
//
// A backend that implements only [scheduler.HistoryStorage], for
// [scheduler.WithHistoryStorage], runs [RunHistory] instead, plus
// [HistoryCleanup] when its CleanupHistory enforces the retention.
//
// # Usage
//
//	func TestStorageContract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, func(tb testing.TB) scheduler.Storage { return newStore(tb) })
//	}
package storagetest
