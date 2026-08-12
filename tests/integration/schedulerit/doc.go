// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package schedulerit exercises service/scheduler end to end against live
// MongoDB and Redis.
//
// The unit tests for service/scheduler run against the in-memory storage, which
// serializes every operation behind one mutex and ignores the context it is
// handed. That makes them fast and deterministic, and it also makes them blind
// to the two things the scheduler actually depends on in production: that a
// backend enforces the compare-and-swap the correctness argument rests on, and
// that a document survives the round trip through a real driver.
//
// Both blind spots have already cost this package. Redis reads dropped every
// field held in the embedded TaskSummary, so tasks came back with a zero status
// — neither active nor running, and therefore invisible to both dispatch and
// stale recovery. The Mongo indexes named fields no document carried, so
// due-task lookups fell back to collection scans and History sorted on a
// missing field. Neither is expressible in memory, and the integration tests
// that would have caught them were being skipped for want of a live backend.
//
// What only appears here:
//
//   - At-most-once execution across instances. Two schedulers sharing one
//     storage must not both run the same occurrence. Leadership is only a
//     throughput optimization; the guarantee comes from Storage.ClaimRun's
//     atomic compare-and-swap, which does not exist until a real server
//     arbitrates it.
//
//   - Round-trip fidelity. A task's status, schedule and next-run time have to
//     survive being written as a document and read back through the driver.
//     The in-memory store hands back the very struct it was given.
//
//   - Crash recovery. A task stranded in TaskStatusRunning by a process that
//     died must be reclaimed on the next start, which means the previous
//     instance's state has to be durable in the first place.
//
//   - Leadership gating. A follower must dispatch nothing at all while a leader
//     exists, and start dispatching once it takes over.
//
// Every fixture skips rather than fails when its backend is unreachable, so a
// machine without the compose stack still gets a green build. See
// tests/integration/README.md.
package schedulerit
