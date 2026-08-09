// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package leadelectit holds the invariant checker the data/leadelect integration suite runs its
// scenarios through.
//
// The package itself is machinery: an Observer that polls a set of electors on a fixed cadence and
// records what each one claimed, plus the two safety properties that recording is checked against.
// The scenarios that drive the elections — steady state, graceful handover, abrupt loss of the
// broker — live in the _test.go files beside it.
//
// # Why a live broker
//
// The unit tests run against an embedded NATS server and assert on the pieces in isolation: the
// freshness bound demotes a stale leader, a fencing token is gated on it, Stop honors its budget.
// What they cannot reach is the property the package exists to provide — that across several
// processes competing for the same key, through a real broker, over real time, at most one of them
// ever believes it is the leader.
//
// Three things only appear here:
//
//   - Server-side key expiry. A holder that dies without resigning releases the lease only when
//     JetStream ages the key out. Nothing in the unit tests waits for that; it is the whole
//     mechanism behind "a lock always has a TTL".
//   - Fencing tokens across real terms. The token is a KV revision, so its monotonicity across a
//     failover is a property of the server's sequencing, not of the code that reads it.
//   - Overlap. Mutual exclusion is a statement about two nodes at one instant, which needs two
//     nodes.
//
// # What sampling can and cannot show
//
// The Observer polls; it does not trace. A round in which two nodes both claim leadership is proof
// that mutual exclusion broke. A run of clean rounds is evidence, not proof, that it held — an
// overlap shorter than the polling interval goes unseen. The interval is therefore set well below
// the lease lifetime, and the failover scenarios sample hardest exactly when a handover is in
// flight, which is the only window in which overlap is plausible.
package leadelectit
