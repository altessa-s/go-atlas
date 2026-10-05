// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest checks scheduler storage contracts across backends.
//
// [Run] executes every contract as a subtest on a fresh store per contract;
// each contract is also exported on its own (FinishRun, RenewRun, ...).
//
// # Usage
//
//	func TestStorageContract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, func(tb testing.TB) scheduler.Storage { return newStore(tb) })
//	}
package storagetest
