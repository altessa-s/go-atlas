// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package bloom provides a Bloom filter implementation for probabilistic existence checks.
// Bloom filters are space-efficient but do not support deletion. Use periodic rebuilds
// when the underlying dataset changes.
//
// Example:
//
//	storage := memory.New(memory.WithExpectedItems(100000))
//	filter := bloom.New(storage)
//	filter.Add(ctx, "user:123")
//	exists, _ := filter.MightExist(ctx, "user:123") // true
package bloom
