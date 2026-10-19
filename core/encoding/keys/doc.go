// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package keys builds unambiguous keys from ordered tuples of parts.
//
// Joining parts with a separator lets two tuples share a key ("a:b" + "c" and "a" + "b:c"). [Compose] frames every part with its
// length instead, so distinct tuples always give distinct strings and the key stays readable; [Digest] hashes length-prefixed parts
// with SHA-256 into a fixed-size key for long or binary input (request fingerprints, cache keys of long queries). Both layouts are
// frozen, since callers persist the keys.
//
//	k := keys.Compose("transport", service, idempotencyKey) // "9:transport..."
//	fp := keys.Digest([]byte(method), body)                 // 32 bytes
package keys
