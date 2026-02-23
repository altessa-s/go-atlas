// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package cuckoo provides a Cuckoo filter implementation for probabilistic existence checks.
// Unlike Bloom filters, Cuckoo filters support deletion of individual items.
//
// Example:
//
//	storage := memory.New(memory.WithCapacity(100000))
//	filter := cuckoo.New(storage)
//	filter.Add(ctx, "user:123")
//	exists, _ := filter.MightExist(ctx, "user:123") // true
//	filter.Delete(ctx, "user:123")
//	exists, _ = filter.MightExist(ctx, "user:123") // false
package cuckoo
