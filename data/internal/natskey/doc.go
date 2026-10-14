// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package natskey maps arbitrary logical keys onto NATS JetStream KeyValue
// keys.
//
// A KeyValue key is part of a NATS subject, so the server accepts only the
// bytes [-/_=.a-zA-Z0-9], no leading or trailing dot and no empty subject
// token. Callers build keys from colons, IPv6 addresses, URL paths and
// arbitrary user input, which the server rejects with "nats: invalid key".
// [Encode] is the single translation every NATS KeyValue store applies to a
// logical key before it reaches the bucket.
//
// # Encoding
//
// Letters, digits, '-', '_', '/' and every dot that neither starts nor ends
// the key nor follows another dot are kept. Every other byte, '=' included, is
// written as '=' followed by two upper-case hex digits. The empty key becomes
// "=", which no other key produces. The mapping works on bytes, so Unicode and
// malformed UTF-8 round-trip exactly, and it is injective: [Decode] inverts it.
//
//	idk:POST:/orders:123 → idk=3APOST=3A/orders=3A123
//	a=3Ab                → a=3D3Ab
//	.leading.            → =2Eleading=2E
//	a..b                 → a.=2Eb
//
// # Compatibility
//
// A key that was already valid and contains no '=' maps to itself, so data
// stored before the encoding was introduced stays reachable. A key containing
// '=' maps to a different physical key than the raw one it was stored under
// before; such data has to be migrated or the store pointed at a fresh bucket.
//
// # Length
//
// The physical key travels in the protocol control line together with the
// bucket subject prefix, the reply inbox and the command. [Encode] rejects a
// key whose encoding exceeds [MaxLen] bytes with [ErrKeyTooLong] before any
// network I/O; keys are never truncated or hashed.
package natskey
