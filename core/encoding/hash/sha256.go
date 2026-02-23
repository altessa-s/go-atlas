// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"unsafe"
)

// SHA256HexBytes returns the lowercase hex-encoded SHA-256 digest of b.
// A nil or empty slice produces the digest of the empty message
// (e3b0c44298fc1c149afbf4c8996fb924...).
//
// The returned string is always 64 hex characters long.
//
// This function is safe for concurrent use because it allocates a fresh
// hash state on every call.
//
// WARNING: This function does not use a salt. Do NOT use it for hashing
// passwords or other low-entropy secrets — use bcrypt/scrypt/argon2 instead.
// See [SHA256HexWithSalt] for a salted variant.
func SHA256HexBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// SHA256HexString returns the lowercase hex-encoded SHA-256 digest of s.
// It is functionally equivalent to [SHA256HexBytes]([]byte(s)) but avoids
// allocating a copy of s by using a zero-copy string-to-[]byte conversion.
//
// An empty string produces the same digest as a nil byte slice in
// [SHA256HexBytes]. The returned string is always 64 hex characters long.
//
// This function is safe for concurrent use.
//
// WARNING: This function does not use a salt. Do NOT use it for hashing
// passwords or other low-entropy secrets — use bcrypt/scrypt/argon2 instead.
// See [SHA256HexStringWithSalt] for a salted variant.
//
// # Safety of Zero-Copy Conversion
//
// This function uses unsafe.StringData and unsafe.Slice for zero-allocation
// string-to-[]byte conversion. This is safe because:
//
//  1. Read-Only Access: sha256.Sum256 only reads from the byte slice; it never
//     modifies the underlying data. This preserves Go's string immutability guarantee.
//
//  2. Bounded Lifetime: The byte slice exists only for the duration of the hash
//     computation. It is not stored, returned, or accessible after the function returns.
//
//  3. Go 1.20+ Functions: unsafe.StringData and unsafe.Slice are the officially
//     supported functions for this conversion pattern since Go 1.20.
//
//  4. Empty String Safety: Empty strings are handled separately to avoid creating
//     a slice from potentially invalid string data pointers.
//
// #nosec G103 -- zero-copy for read-only hashing, see safety documentation above
func SHA256HexString(s string) string {
	if s == "" {
		return SHA256HexBytes(nil)
	}
	// Zero-allocation conversion from string to []byte for hashing.
	// Safe because sha256.Sum256 only reads from the slice.
	b := unsafe.Slice(unsafe.StringData(s), len(s))
	return SHA256HexBytes(b)
}

// SHA256HexWithPrefix returns the concatenation of prefix and the lowercase
// hex-encoded SHA-256 digest of s (i.e. prefix + hex(SHA-256(s))).
// This is useful for constructing namespaced cache keys or storage paths
// where the prefix acts as a domain separator.
//
// An empty s is hashed normally (the digest of the empty message is appended).
// The prefix is prepended verbatim and is not included in the hash input.
//
// This function is safe for concurrent use.
//
// WARNING: This function does not use a salt. Do NOT use it for hashing
// passwords or other low-entropy secrets — use bcrypt/scrypt/argon2 instead.
//
// # Safety
//
// See [SHA256HexString] documentation for zero-copy conversion safety invariants.
//
// #nosec G103 -- zero-copy for read-only hashing
func SHA256HexWithPrefix(prefix, s string) string {
	if s == "" {
		return prefix + hex.EncodeToString(sha256.New().Sum(nil))
	}
	// Zero-allocation conversion from string to []byte for hashing.
	// Safe because sha256.Sum256 only reads from the slice.
	b := unsafe.Slice(unsafe.StringData(s), len(s))
	sum := sha256.Sum256(b)
	return prefix + hex.EncodeToString(sum[:])
}

// SHA256HexWithSalt returns the lowercase hex-encoded SHA-256 digest of the
// concatenation salt || data (i.e. hex(SHA-256(salt || data))).
// Prepending the salt prevents precomputed rainbow-table attacks on the
// resulting digest.
//
// Both data and salt may be nil or empty; in that case the missing part is
// simply omitted from the hash input.
//
// The returned string is always 64 hex characters long.
//
// This function is safe for concurrent use.
//
// WARNING: This is NOT suitable for password hashing — use bcrypt/scrypt/argon2
// for that purpose. This function is intended for salting identifiers, tokens,
// or other data that needs deterministic yet domain-separated digests.
//
// See [SHA256HexStringWithSalt] for the string variant.
func SHA256HexWithSalt(data, salt []byte) string {
	h := sha256.New()
	h.Write(salt)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// SHA256HexStringWithSalt returns the lowercase hex-encoded SHA-256 digest of
// the concatenation salt || s (i.e. hex(SHA-256(salt || s))).
// It is the string variant of [SHA256HexWithSalt], using a zero-copy
// string-to-[]byte conversion to avoid allocating copies of salt and s.
//
// Both s and salt may be empty; empty values are simply omitted from the
// hash input. The returned string is always 64 hex characters long.
//
// This function is safe for concurrent use.
//
// WARNING: This is NOT suitable for password hashing — use bcrypt/scrypt/argon2
// for that purpose.
//
// # Safety of Zero-Copy Conversion
//
// See [SHA256HexString] documentation for zero-copy conversion safety invariants.
//
// #nosec G103 -- zero-copy for read-only hashing
func SHA256HexStringWithSalt(s, salt string) string {
	h := sha256.New()
	if salt != "" {
		h.Write(unsafe.Slice(unsafe.StringData(salt), len(salt)))
	}
	if s != "" {
		h.Write(unsafe.Slice(unsafe.StringData(s), len(s)))
	}
	return hex.EncodeToString(h.Sum(nil))
}
