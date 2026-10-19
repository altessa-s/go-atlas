// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueue

import (
	"context"
	"time"
)

// Storage persists retryable work, one [Item] per key.
//
// Instants are supplied by the caller (its clock), never by the backend. A
// missing item is never an error: a transition on it is a no-op. Every method
// that takes a generation applies only while the stored item is at that
// generation (see [Item.Generation]); a stale call is a no-op. Methods that
// report a bool report whether this call made the transition, so of several
// racing callers exactly one observes true.
//
// [Storage.ListDue] is a read, not a lease: two sweepers may list the same item.
// Correctness under concurrency comes from the fenced transitions, which apply
// at most once per generation and age.
type Storage[P any] interface {
	// Mark stores item as a fresh pending item (no attempts, no flag, no
	// expiry or resolution) unless an item for item.Key already exists, and
	// returns the stored item. It is insert-only: a repeat keeps the original
	// Since and Payload, and an expired or resolved item keeps its state. Of
	// concurrent first marks exactly one inserts; the others return its item.
	Mark(ctx context.Context, item Item[P]) (Item[P], error)

	// Reopen moves the item of item.Key to item.Generation (newer work), or
	// reopens an expired or resolved item of the same generation: pending, with
	// item's Payload (replacing the stored one whole), Since and NextAttemptAt,
	// no attempts, and no expiry, resolution or flag. An item already at a newer
	// generation, or pending at the same one, is returned unchanged — the caller
	// compares generations. A missing item is inserted as [Storage.Mark] does.
	Reopen(ctx context.Context, item Item[P]) (Item[P], error)

	// ListDue returns up to limit pending items whose NextAttemptAt is not
	// after now, oldest NextAttemptAt first (ties by key), so a sweep that
	// reschedules what it cannot complete reaches every item in turn. A limit
	// that is not positive means [DefaultListLimit].
	ListDue(ctx context.Context, now time.Time, limit int) ([]Item[P], error)

	// WakeDue makes every pending item scheduled after now due at now (the
	// condition the work waited for may have changed). It counts no attempt.
	WakeDue(ctx context.Context, now time.Time) error

	// Reschedule counts a failed attempt and moves the next one to next. Only
	// a pending item moves: an expired or resolved one is never revived.
	Reschedule(ctx context.Context, key string, generation int64, next time.Time) error

	// Expire stops retrying a pending item, keeping it for observability. It
	// applies only while the item still has the age (since) the caller
	// observed, so a stale attempt cannot expire a requeued item, and reports
	// whether the item expired.
	Expire(ctx context.Context, key string, generation int64, since, at time.Time) (bool, error)

	// Resolve marks a pending or expired item resolved at at and drops its
	// expiry and flag. It is no longer listed as due but is kept until the
	// storage's retention purges it. An already resolved item keeps its
	// original ResolvedAt.
	Resolve(ctx context.Context, key string, generation int64, at time.Time) error

	// Flag marks a pending item as needing attention for reason and reports
	// whether the flag is new or its reason changed, so an alert raised on
	// true fires once per reason. Only a pending item is flagged.
	Flag(ctx context.Context, key string, generation int64, reason string) (bool, error)

	// ClearFlag drops the flag of the item in any state.
	ClearFlag(ctx context.Context, key string, generation int64) error

	// DeleteObsolete removes the item unless it is at a generation newer than
	// generation, and reports whether such a newer item exists (the caller's
	// snapshot of the work is then stale).
	DeleteObsolete(ctx context.Context, key string, generation int64) (newer bool, err error)

	// Delete removes the item whatever its generation.
	Delete(ctx context.Context, key string) error

	// ListByState returns up to limit items in state, oldest Since first (ties
	// by key). A limit that is not positive means [DefaultListLimit].
	ListByState(ctx context.Context, state State, limit int) ([]Item[P], error)

	// Requeue moves expired items back to pending, due at now with a fresh age
	// (Since = now), no attempts and no expiry, and returns how many moved.
	// With no keys every expired item moves. Generations and flags are kept.
	Requeue(ctx context.Context, now time.Time, keys ...string) (int, error)
}
