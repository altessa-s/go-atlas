// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the conformance kit for [storages.Storage]
// backends. [Run] runs parallel contracts on fresh stores, covering
// transport-sized replay records and protection against completed-record takeover.
//
// # Usage
//
//	func TestStorageContract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, func(tb testing.TB) storages.Storage {
//			store, err := memory.New()
//			require.NoError(tb, err)
//			return store
//		})
//	}
package storagetest
