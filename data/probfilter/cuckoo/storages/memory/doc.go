// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides an in-memory Cuckoo filter storage implementation
// using the seiflotfy/cuckoofilter library.
//
// Example:
//
//	storage := memory.New(memory.WithCapacity(100000))
//	defer storage.Close()
package memory
