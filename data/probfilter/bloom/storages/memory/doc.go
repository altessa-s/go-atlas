// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-memory Bloom filter storage implementation
// using the bits-and-blooms/bloom library.
//
// Example:
//
//	storage := memory.New(
//	    memory.WithExpectedItems(100000),
//	    memory.WithFalsePositiveRate(0.01),
//	)
//	defer storage.Close()
package memory
