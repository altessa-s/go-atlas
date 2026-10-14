// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotencyit runs data/idempotency against live backends: one
// storage contract suite over the Redis, MongoDB and NATS JetStream storages,
// then the Keeper and the HTTP idempotency middleware on top of each.
//
// The unit tests of the storages run against miniredis, an embedded NATS
// server and a MongoDB fake or nothing at all. What they cannot show is
// whether the guards hold on the real server:
//
//   - The Redis storage fences Complete, Steal and Release with Lua scripts
//     that compare the stored bytes. Lua string semantics, binary-safe
//     comparison and the PX argument passed as a string are exactly where
//     miniredis and Redis may differ, so every branch of both scripts runs
//     here: a match, a mismatch and a missing key.
//   - Expiry is the server's. A per-call lock TTL and the storage TTL must
//     both age a key out on the server clock, and a holder whose lock expired
//     must then be refused, not allowed to overwrite the next holder.
//   - Exactly one of many concurrent callers acquires a key, and exactly one
//     of many concurrent stealers or releasers wins the compare-and-set.
//
// Tests skip rather than fail when a server is unreachable. The MongoDB
// backend runs only when MONGO_URI is set, so the suite never writes to an
// unrelated server listening on the compose port. See
// tests/integration/README.md.
package idempotencyit
