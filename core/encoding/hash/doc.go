// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package hash provides small, stdlib-only helpers for producing
// deterministic, hex-encoded SHA-256 digest strings.
//
// All stateless functions are safe for concurrent use. They are intended for
// non-cryptographic use cases such as cache keys, content addressing, and
// deduplication tokens. None of these functions are suitable for password
// hashing; use bcrypt, scrypt, or argon2 instead.
//
// For streaming or incremental hashing (e.g. while piping through an
// [io.MultiWriter]), use [NewSHA256Hasher]. It returns a [*SHA256Hasher] that
// implements [Hasher] ([io.Writer] + [SHA256Hasher.SumHex]). [HexSum] converts
// any stdlib [hash.Hash] to a lowercase hex string without resetting state.
//
// Salted variants ([SHA256HexWithSalt], [SHA256HexStringWithSalt]) prepend a
// caller-supplied salt to the input before hashing, providing domain separation
// and resistance to precomputed rainbow-table attacks.
//
// Example:
//
//	key := hash.SHA256HexWithPrefix("tokens:", token)
package hash
