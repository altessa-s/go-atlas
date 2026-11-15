// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-memory Cuckoo filter storage implementation:
// a partial-key cuckoo filter with 4-slot buckets and 8-bit fingerprints.
//
// A failed insert ([ErrFilterFull]) undoes its relocation walk, so it never
// evicts a previously added value: every added and not deleted value keeps
// answering true. Deleting a value that was never added can still remove the
// fingerprint of a colliding member — only delete values that were added.
//
// Example:
//
//	storage := memory.New(memory.WithCapacity(100000))
//	defer storage.Close()
package memory
