// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package brokerit holds the delivery recorder the transport/broker integration suite runs its
// scenarios through.
//
// The package itself is machinery: a Recorder that stands in for a subscriber handler, records
// every delivery it is handed and reacts to it the way the scenario asks — ack, nak, terminate,
// stall or panic. The scenarios that publish, consume, restart and shut down live in the _test.go
// files beside it.
//
// # Why a live broker
//
// The unit tests for transport/broker drive the NATS provider through fakes: an acker that records
// calls, a consume context that emulates a drain. What they cannot reach is what JetStream does
// with those calls — and that is the whole contract of a message broker:
//
//   - Redelivery. A Nak, a NakWithBackOff, an expired ack wait and a handler panic all end in the
//     server sending the message again, on the server's schedule. A Term and an exhausted
//     MaxDeliver end in it never being sent again.
//   - In-progress extension. A heartbeat resets the server's ack timer; whether a slow handler
//     escapes redelivery depends on the server honoring it.
//   - At-least-once across restarts. A durable consumer remembers what was acknowledged; a
//     consumer that dies mid-stream must leave nothing unaccounted for.
//   - Idempotency over real redelivery. The middleware collapses duplicates through a shared
//     store, so the concurrent case needs two consumer instances racing for the same key.
//
// # What the recorder proves
//
// Every handler invocation is recorded with its payload and the time it arrived, so a redelivery
// is two entries for one payload rather than a count that came out high, and a backoff is the gap
// between those two entries as the handler saw it.
package brokerit
