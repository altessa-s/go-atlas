// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package outboxstore provides MongoDB implementation of outbox.Store.
// Persists outbox events with automatic index creation and status tracking.
//
// # Clock-skew safety
//
// All time-based decisions — the retry gate, the lock lease, expiration, and
// cleanup — are evaluated against the MongoDB server clock via the $$NOW
// aggregation variable, never against a caller's wall clock. The lock timestamp
// (locked_on) and the retry timestamp (last_attempt_on) are likewise stamped with
// $$NOW. As a result, multiple service instances with skewed clocks cannot
// disagree on whether an event is due, stuck, or expired: there is one clock, the
// primary's. Callers pass only policy durations (retry interval, lock TTL,
// cleanup age); see outbox.Store.
//
// Timestamps are stored as BSON Date (not Unix integers); optional ones
// (published_at, last_attempt_on, locked_on, expires_at) are absent when unset.
//
// Example:
//
//	store, _ := outboxstore.New(db,
//	    outboxstore.WithCollectionName("custom_outbox"),
//	)
//	ob := outbox.New(store, handler)
package outboxstore
