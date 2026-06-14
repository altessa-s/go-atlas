// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag

import "crypto/rand"

// Random returns a fresh weak entity-tag backed by a cryptographically random
// opaque token (Go's crypto/rand.Text, ~130 bits of base32). Every call returns
// a different tag, so it is a cheap change/version token: generate one on each
// model update and persist it, instead of hashing the representation.
//
// It is weak by construction (RFC 7232 §2.3) because the value is not derived
// from the representation's bytes — it asserts only "this is a different
// version", never byte-for-byte identity. Generating it is far cheaper than a
// content hash, at the cost of the guarantees listed below.
//
// # When NOT to use
//
//   - As an HTTP cache validator (ETag + If-None-Match). Identical content gets
//     a new tag on every regenerate, so caches miss and clients re-download
//     unchanged bytes. Use [Generator.Hash] / [Generator.HashString] /
//     [Generator.HashReader] for content-addressed caching.
//   - For byte-range revalidation (If-Range, 206 Partial Content). RFC 7232
//     §2.1 forbids weak validators there; serving ranges against a tag that does
//     not track the bytes can corrupt reassembly. Ranges need a STRONG content
//     tag from a [Generator].
//   - Where the tag must be reproducible from content: cross-replica agreement,
//     retry idempotency, or deduplication. Two calls never agree, so the value
//     cannot be recomputed — it only works when stored alongside the resource.
//
// # When to use
//
// Optimistic concurrency control (HTTP If-Match, gRPC/AIP-154 mutation
// validation) and cheap resource versioning, where you only need to detect
// "did this change since I read it" and the tag is persisted with the resource.
// The caller is then responsible for regenerating it on every content mutation;
// if the bytes can change without a new Random tag, the validator goes stale.
func Random() Tag {
	return Weak(rand.Text())
}
