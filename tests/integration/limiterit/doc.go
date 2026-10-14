// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package limiterit runs the data/limiters storage contract against live
// backends — the Redis provider on a real server and the NATS provider on a
// real JetStream KeyValue bucket — with the in-memory provider as the
// reference every shared backend is compared against. The tokenbucket and
// budget limiters are then driven on top of each storage.
//
// The unit tests of the Redis provider run its Lua script on miniredis, whose
// script engine and clock are emulations; the NATS provider's unit tests run
// on an embedded server inside one process. What only shows here:
//
//   - The sliding window on a real server: exactly limit admissions per
//     window, entries expiring one by one as the window slides, and the
//     Remaining and Reset values the script and the CAS loop compute.
//   - Admission under contention: many goroutines, and two provider instances
//     on their own connections standing in for two service replicas, hammering
//     one key must never admit more than the limit in total.
//
// Tests skip rather than fail when a server is unreachable, so a machine
// without the compose stack still gets a green build. Every test works under
// its own Redis key prefix or NATS bucket, removed on cleanup. See
// tests/integration/README.md.
package limiterit
