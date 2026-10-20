// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the conformance kit for [storages.MultiStorage]
// backends. [Run] executes every contract — an admission takes a slot on
// every key, a refusal takes none and names the first refusing key,
// duplicate keys are rejected, and concurrent calls over overlapping keys
// never over-admit or take a partial set of slots — as parallel subtests on
// fresh stores.
//
// The keys of one call share the hash tag {t}, so the kit also runs against
// a Redis Cluster backend.
//
// # Usage
//
//	func TestMultiStorageContract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, func(testing.TB) storages.MultiStorage {
//			return memory.New()
//		})
//	}
package storagetest
