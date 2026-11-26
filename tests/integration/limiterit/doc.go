// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package limiterit runs the coord/limiters storage contract against live
// Redis, with the in-memory provider as the reference the shared backend is
// compared against. The tokenbucket and budget limiters are then driven on top
// of each storage.
//
// The unit tests of the Redis provider run its Lua script on miniredis, whose
// script engine and clock are emulations. What only shows here:
//
//   - The sliding window on a real server: exactly limit admissions per
//     window, entries expiring one by one as the window slides, and the
//     Remaining and Reset values the script computes.
//   - Admission under contention: many goroutines, and two provider instances
//     on their own connections standing in for two service replicas, hammering
//     one key must never admit more than the limit in total.
//
// Tests skip rather than fail when Redis is unreachable, so a machine without
// the compose stack still gets a green build. Every test works under its own
// Redis key prefix, removed on cleanup. See tests/integration/README.md.
package limiterit
