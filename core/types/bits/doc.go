// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package bits provides generic bit manipulation functions for all integer types.
// It covers single-bit operations, range operations, counting, rotation, and masking.
//
// Example:
//
//	value := uint8(5) // 00000101
//	bits.IsBitSet(value, 0)  // true
//	bits.SetBit(value, 1)    // 00000111 (7)
//	bits.CountSetBits(value) // 2
package bits
