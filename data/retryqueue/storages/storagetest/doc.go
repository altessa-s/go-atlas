// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storagetest is the conformance kit for [retryqueue.Storage]
// backends. [Run] executes every contract — insert-only marking, due order,
// rescheduling, expiry and its age fence, resolution, flags, requeueing,
// generation fencing and reopening, and the exactly-once reports under
// concurrency — as parallel subtests on fresh stores.
//
// The contracts store a [Payload] with one omitempty field, and use
// whole-second UTC instants so that values round-tripped through a backend
// compare equal.
//
// # Usage
//
//	func TestStorageContract(t *testing.T) {
//		t.Parallel()
//		storagetest.Run(t, func(testing.TB) storagetest.Storage {
//			return memory.New[storagetest.Payload]()
//		})
//	}
package storagetest
