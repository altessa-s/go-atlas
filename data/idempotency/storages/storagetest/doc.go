// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the conformance kit for [storages.Storage]
// backends. [Run] executes every contract — currently that a Keeper stores
// and replays a transport-sized replay record intact — as parallel subtests
// on fresh stores.
//
// # Usage
//
//	func TestStorageContract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, func(testing.TB) storages.Storage {
//			return memory.New()
//		})
//	}
package storagetest
