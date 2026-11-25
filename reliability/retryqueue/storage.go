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
// [Storage.ListDue] is a read, not a lease: two sweepers may list and process
// the same item. Use [AttemptStorage] for worker outcomes after reading an
// item. The generation-only mutations on Storage apply to the current item
// of that generation, even after a same-generation Reopen or Requeue.
//
// P must be representable by the chosen backend. Memory snapshots Go values,
// SQL uses the configured codec (JSON by default), and MongoDB inlines a BSON
// struct or struct pointer. Its top-level fields must avoid queue and flag
// fields, including through inline structs, and cannot include an inline map.
type Storage[P any] interface {
	ProducerStorage[P]
	AdminStorage[P]

	// ListDue returns up to limit pending items whose NextAttemptAt is not
	// after now, oldest NextAttemptAt first (ties by key), so a sweep that
	// reschedules what it cannot complete reaches every item in turn. A limit
	// that is not positive means [DefaultListLimit].
	ListDue(ctx context.Context, now time.Time, limit int) ([]Item[P], error)

	// Expire stops retrying a pending item, keeping it for observability. It
	// applies only while the item still has the age (since) the caller
	// observed, so a stale attempt cannot expire a requeued item, and reports
	// whether the item expired.
	Expire(ctx context.Context, key string, generation int64, since, at time.Time) (bool, error)
}

// ProducerStorage inserts work and reopens it when the work changes.
type ProducerStorage[P any] interface {
	// Mark stores item as a fresh pending item (no attempts, no flag, no
	// expiry or resolution) unless an item for item.Key already exists, and
	// returns the stored item. It is insert-only: a repeat keeps the original
	// Since and Payload, and an expired or resolved item keeps its state. Of
	// concurrent first marks exactly one inserts; the others return its item.
	Mark(ctx context.Context, item Item[P]) (Item[P], error)

	// Reopen moves the item of item.Key to item.Generation (newer work), or
	// reopens an expired or resolved item of the same generation: pending, with
	// item's Payload (replacing the stored one whole), Since and NextAttemptAt,
	// no attempts, and no expiry, resolution or flag. At the same generation
	// Since advances by at least one storage tick (see Requeue). An item already
	// at a newer generation or pending at the same one is returned unchanged. The caller
	// compares generations. A missing item is inserted as [ProducerStorage.Mark] does.
	Reopen(ctx context.Context, item Item[P]) (Item[P], error)
}

// AdminStorage inspects and manages the current generation of queued work.
// Workers reporting an observed attempt should use [WorkerStorage].
type AdminStorage[P any] interface {
	// WakeDue makes every pending item scheduled after now due at now (the
	// condition the work waited for may have changed). It counts no attempt.
	WakeDue(ctx context.Context, now time.Time) error

	// Reschedule counts a failed attempt and moves the next one to next. Only
	// a pending item moves: an expired or resolved one is never revived.
	Reschedule(ctx context.Context, key string, generation int64, next time.Time) error

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
	// (Since = now or, when now has not advanced, one storage tick past the
	// previous Since), no attempts and no expiry, and returns how many moved.
	// With no keys every expired item moves. Generations and flags are kept.
	Requeue(ctx context.Context, now time.Time, keys ...string) (int, error)
}

// Attempt identifies the incarnation of a retry item a worker observed.
// Obtain it from [Item.Attempt]. Since changes on Requeue and on a
// same-generation Reopen, while the generation identifies the work revision.
type Attempt struct {
	Key        string
	Generation int64
	Since      time.Time
}

// WorkerStorage provides the reads and age-fenced updates used by retry workers.
// Its mutations have the same behavior as their
// generation-only counterparts, with the same age check as [Storage.Expire].
// A worker can report a stale outcome harmlessly after Reopen or Requeue.
// This is not a lease, and callers that delete and reinsert a key must avoid
// reusing both its generation and age while old workers can still report.
type WorkerStorage[P any] interface {
	ListDue(ctx context.Context, now time.Time, limit int) ([]Item[P], error)
	Expire(ctx context.Context, key string, generation int64, since, at time.Time) (bool, error)
	RescheduleAttempt(ctx context.Context, attempt Attempt, next time.Time) error
	ResolveAttempt(ctx context.Context, attempt Attempt, at time.Time) error
	FlagAttempt(ctx context.Context, attempt Attempt, reason string) (bool, error)
	ClearAttemptFlag(ctx context.Context, attempt Attempt) error
}

// AttemptStorage combines the worker and queue management interfaces.
// Every bundled backend implements it.
type AttemptStorage[P any] interface {
	Storage[P]
	WorkerStorage[P]
}

// Mover is implemented by a [Storage] that can rename an item's key
// atomically (every bundled backend does). It is a separate interface so a
// Storage implementation outside this module keeps compiling; callers assert
// it, or depend on a backend's concrete type.
type Mover interface {
	// Move moves the item of from to the key to, keeping every other field —
	// generation, state, age, attempts, schedule, expiry, resolution, flag and
	// payload — so the item reads under to exactly as it read under from, and
	// from is left without an item. The move is atomic: no other operation
	// observes the item under both keys or under neither.
	//
	// It fails with [ErrItemNotFound] when from has no item and with
	// [ErrItemExists] when to already has one; nothing changes then. Moving an
	// item onto its own key changes nothing (ErrItemNotFound when it has none).
	Move(ctx context.Context, from, to string) error
}
