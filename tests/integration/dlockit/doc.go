// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package dlockit holds the mutual-exclusion checker the data/locks/dlock integration suite runs
// its scenarios through.
//
// The package itself is machinery: a Critical section recorder that timestamps every entry and
// exit, and the overlap check that recording is verified against. The scenarios that contend for
// the locks live in the _test.go files beside it.
//
// # Why a live broker
//
// The unit tests run against an embedded NATS server and assert on the pieces in isolation: a
// released lock stops renewing, a configured ratio reaches the lease, a lock outlives its acquire
// timeout. What they cannot reach is the property the package exists to provide — that when many
// contenders race for one key through a real broker, their critical sections never overlap.
//
// Three things only appear here:
//
//   - Contention. A lock that is never contested is indistinguishable from no lock at all. Only
//     several holders racing for the same key exercise the acquire path that must lose.
//   - Server-side key expiry. A holder that dies without releasing frees the lock only when
//     JetStream ages the key out; that is the whole of "a lock always has a TTL".
//   - Fencing tokens across real handovers. The token is a KV revision, so its monotonicity across
//     an expiry-driven takeover is a property of the server's sequencing.
//
// # What the recording proves
//
// Unlike a poller, the Critical recorder does not sample: every holder reports the instant it
// entered and the instant it left, so an overlap of any duration is caught. What the recording
// cannot see is a holder that believes it holds the lock without telling the recorder — which is
// why the scenarios drive every acquisition through it.
//
// Timestamps come from one process and one clock, so comparing them is sound here. The same
// recording taken across machines would not be.
package dlockit
