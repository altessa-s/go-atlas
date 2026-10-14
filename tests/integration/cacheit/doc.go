// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package cacheit runs data/cache against a live Redis: the Redis storage on
// its own, then the Cache facade over it.
//
// The unit tests of the Redis storage run against miniredis, whose expiry is
// driven by the test rather than by a clock. Here keys age out on the
// server's own timer, values of several megabytes cross a real connection,
// and the facade's singleflight and negative caching are observed through
// what actually lands in Redis:
//
//   - Save with a TTL expires on the server, NoTTL never expires, and the
//     facade applies its default TTL when the caller passes none.
//   - Many concurrent GetWithFallback calls for one missing key run the
//     fallback once and store its result once.
//   - A fallback reporting "not found" leaves a negative entry that answers
//     further lookups without running the fallback, until it expires.
//
// The storage needs no Lua: every operation is a single command or a
// MULTI/EXEC pipeline. Tests skip rather than fail when Redis is unreachable.
// See tests/integration/README.md.
package cacheit
