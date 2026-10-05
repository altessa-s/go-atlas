// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the contract suite for [scheduler.Storage]
// implementations. Every bundled backend runs it, and a custom backend should
// too: the scheduler's correctness rests on the atomic compare-and-swap writes
// and orderings it checks, which the in-memory backend gets for free and a real
// database must earn.
//
// Each function takes a fresh, isolated store (FinishRun and ReplaceTaskIf
// tolerate shared state; the others expect an empty store) and fails the test
// on any contract violation.
//
// # Usage
//
//	func TestContract(t *testing.T) {
//		storagetest.FinishRun(t, newStore(t))
//		storagetest.ReplaceTaskIf(t, newStore(t))
//		storagetest.ClaimRun(t, newStore(t))
//		storagetest.DueTasks(t, newStore(t))
//		storagetest.Identity(t, newStore(t))
//		storagetest.Pagination(t, newStore(t))
//		storagetest.History(t, newStore(t))
//	}
package storagetest
