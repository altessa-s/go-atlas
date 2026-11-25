// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the contract suite for [outbox.Storage]
// implementations. Every bundled backend runs it, and a custom backend should
// too: delivery rests on oldest-first fetches that hand each event to one
// dispatcher, the lock-token fence of UpdateEvents, and the backend-clock
// predicates of the retry, unlock, expiry and retention sweeps.
//
// [Run] executes the common contracts as parallel subtests on fresh stores.
// [RunWithOrderKeys] also verifies the declared ordering-key format. [Watch] covers the
// optional [outbox.Watcher] capability on stores that implement it.
//
// # Precision
//
// Fixtures use millisecond instants, the precision every bundled backend keeps
// (MongoDB's BSON date). The contracts drive the store directly, without
// business transactions; saving inside one is covered by the integration
// suites in tests/integration/outboxit.
//
// # Usage
//
//	func TestStorageContract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, func(tb testing.TB) outbox.Storage { return newStore(tb) })
//	}
package storagetest
