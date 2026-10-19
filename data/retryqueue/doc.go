// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package retryqueue defines a durable retry queue: one record of retryable
// work per key, swept by a periodic job until the work completes, expires or
// is dropped.
//
// A record ([Item]) is pending, expired or resolved. A sweep lists the pending
// records that are due ([Storage.ListDue]), attempts each, and records the
// outcome: [Storage.Reschedule] pushes a failed attempt back, [Storage.Expire]
// stops retrying work older than the caller's maximum age (keeping the record
// for operators), [Storage.Resolve] closes completed work, and
// [Storage.Delete] drops work that no longer exists. [Storage.WakeDue] makes
// every pending record due at once when the condition the work waited for may
// have changed, and [Storage.Requeue] gives expired records a fresh start.
//
// # Fencing
//
// A record carries a generation, the revision of the work it was computed
// from. [Storage.Reopen] moves a record to a newer generation only, and every
// other transition that takes a generation applies only while the record is
// still at it: a worker holding an older snapshot of the work cannot close,
// reopen or delete newer work. [Storage.Expire] is additionally fenced by the
// record's age, so an attempt that read the record before an operator requeued
// it cannot expire the requeued record. [Storage.Mark] is insert-only, so a
// redelivered first attempt keeps the payload the first one pinned.
//
// Listing is a read, not a lease: concurrent sweepers may attempt the same
// record, and the fenced transitions decide which outcome sticks. A transition
// that reports a bool reports whether this call made it, so of several racing
// callers exactly one observes true.
//
// # Backends
//
// The storages/memory backend is an in-process reference implementation; the
// storages/mongo backend persists records in a MongoDB collection. Both pass
// the storages/storagetest conformance kit.
//
// # Usage
//
//	queue, err := retryqueuemongo.New[Valuation](db.Collection("valuations"))
//	if err != nil {
//		return err
//	}
//	if err := queue.EnsureIndexes(ctx); err != nil {
//		return err
//	}
//	due, err := queue.ListDue(ctx, now, 0)
//	for _, item := range due {
//		if err := attempt(ctx, item); err != nil {
//			_ = queue.Reschedule(ctx, item.Key, item.Generation, now.Add(retryInterval))
//			continue
//		}
//		_ = queue.Resolve(ctx, item.Key, item.Generation, now)
//	}
package retryqueue
