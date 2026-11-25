// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package retryqueue defines a durable retry queue: one record of retryable
// work per key, swept by a periodic job until the work completes, expires or
// is dropped.
//
// A record ([Item]) is pending, expired or resolved. A sweep lists the pending
// records that are due ([Storage.ListDue]), attempts each, and records the
// outcome: [WorkerStorage.RescheduleAttempt] pushes a failed attempt back, [Storage.Expire]
// stops retrying work older than the caller's maximum age (keeping the record
// for operators), [WorkerStorage.ResolveAttempt] closes completed work, and
// [AdminStorage.Delete] drops work that no longer exists. [AdminStorage.WakeDue] makes
// every pending record due at once when the condition the work waited for may
// have changed, and [AdminStorage.Requeue] gives expired records a fresh start.
// A backend that implements [Mover] also moves a record to another key
// atomically, every other field kept ([ErrItemNotFound], [ErrItemExists]).
//
// Producers can depend on [ProducerStorage] and administrators on [AdminStorage].
// The payload restrictions of each backend are documented on [Storage].
//
// # Fencing
//
// A record carries a generation, the revision of the work it was computed
// from. [ProducerStorage.Reopen] moves a record to a newer generation only, and every
// other transition that takes a generation applies only while the record is
// still at it: a worker holding an older snapshot of the work cannot close,
// reopen or delete newer work. [Storage.Expire] and the worker mutations on
// [AttemptStorage] also compare the stored age. This protects work revived
// within the same generation by Requeue or Reopen. [ProducerStorage.Mark] is insert-only, so a
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
// storages/mongo backend persists records in a MongoDB collection and the
// storages/sqldb backend in a PostgreSQL, MySQL or MariaDB table. Each
// implements [Mover] and [AttemptStorage] and passes the storages/storagetest conformance kit.
// factory/reliability/retryqueue builds a backend from config/retryqueue.
// SQL PurgeExpired(ctx) (int64, error) drains resolved rows past retention;
// a lazy background loop runs at WithPurgeInterval until Close.
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
//			_ = queue.RescheduleAttempt(ctx, item.Attempt(), now.Add(retryInterval))
//			continue
//		}
//		_ = queue.ResolveAttempt(ctx, item.Attempt(), now)
//	}
package retryqueue
