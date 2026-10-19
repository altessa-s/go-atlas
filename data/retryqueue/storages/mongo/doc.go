// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides the MongoDB-backed [retryqueue.Storage]: one document
// per item in a caller-chosen collection, with the key as _id and the payload
// inlined next to the store's fields.
//
// # Documents
//
// The store owns the fields _id, since, attempts, nextAttemptAt, state,
// expiredAt, resolvedAt and generation, plus the two flag fields (flagged and
// flagReason by default, renamed with [WithFlagField] and
// [WithFlagReasonField] to keep an existing collection's wire format). The
// optional dates and the flag are absent until set. A document without a
// generation (written before generations were used) reads as generation 0.
// The payload must be a struct (or struct pointer) without an inlined map, so
// its fields are known statically; [New] rejects one whose fields collide
// with the store's. Reopen replaces the whole document in one write.
//
// # Indexes
//
// [Store.EnsureIndexes] creates a partial index on pending items by next
// attempt (ListDue, WakeDue), an index by state and age (ListByState,
// Requeue), and, with [WithResolvedRetention], a TTL index that purges
// resolved items after the retention.
//
// # Concurrency
//
// Every transition is a single conditional write, so it applies atomically
// and reports whether it matched. Mark is an insert-only upsert; a concurrent
// first mark that loses the insert race with a duplicate-key error is repeated
// once and returns the winner's item. Inside a transaction it is not repeated:
// the error has aborted the transaction.
//
// # Usage
//
//	queue, err := mongo.New[Valuation](db.Collection("valuations"),
//		mongo.WithResolvedRetention(7*24*time.Hour))
//	if err != nil {
//		return err
//	}
//	if err := queue.EnsureIndexes(ctx); err != nil {
//		return err
//	}
package mongo
