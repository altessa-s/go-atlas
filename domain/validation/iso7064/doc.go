// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package iso7064 provides check-digit validation using the ISO 7064
// MOD 11-10 algorithm for 9-digit numeric identifiers.
//
// [Mod11_10] returns both the validation result and an error for malformed
// input. [IsValidMod11_10] is a convenience wrapper that returns false on
// any error.
//
// Both functions accept string or int64 via a generic type parameter. When
// using int64, leading zeros are not preserved; if your identifier can have
// leading zeros, pass it as a string.
//
// Example:
//
//	valid := iso7064.IsValidMod11_10("123456788")
//	valid, err := iso7064.Mod11_10("123456788")
package iso7064
