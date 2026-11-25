// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueue

import "time"

// State is the lifecycle state of an [Item].
type State string

// Item states. Only a pending item is listed by [Storage.ListDue] and moved by
// [AdminStorage.Reschedule], [AdminStorage.WakeDue], [Storage.Expire] and [AdminStorage.Flag].
const (
	// StatePending marks work that is retried until it is resolved, expires or
	// is deleted.
	StatePending State = "pending"
	// StateExpired marks work that outlived its maximum age. It is kept for
	// observability until an operator requeues or deletes it.
	StateExpired State = "expired"
	// StateResolved marks completed work. It is kept (so a late duplicate of
	// the work can read what the first attempt pinned in the payload) until
	// the storage's retention purges it.
	StateResolved State = "resolved"
)

// DefaultListLimit caps [Storage.ListDue] and [AdminStorage.ListByState] when the
// caller passes a limit that is not positive.
const DefaultListLimit = 200

// Item is one unit of retryable work, keyed by Key: a storage holds at most one
// item per key.
//
// Generation fences stale work: every transition that takes a generation
// applies only while the stored item is still at that generation, so a worker
// holding an older snapshot of the work never closes, reopens or deletes newer
// work. Since fences [Storage.Expire] and [AttemptStorage] mutations across
// [AdminStorage.Requeue] and a same-generation [ProducerStorage.Reopen]. An item stored before generations were used reads as
// generation 0.
type Item[P any] struct {
	// Key identifies the work; it is the storage's primary key.
	Key string
	// Generation is the revision of the work the item was computed from. It
	// moves forward only, through [ProducerStorage.Reopen].
	Generation int64
	// State is pending, expired or resolved.
	State State
	// Since is when the work was first found pending; the caller measures the
	// maximum age from it. [AdminStorage.Requeue] and [ProducerStorage.Reopen] reset it.
	Since time.Time
	// Attempts counts the failed attempts recorded by [AdminStorage.Reschedule].
	Attempts int
	// NextAttemptAt is when the work becomes due again.
	NextAttemptAt time.Time
	// ExpiredAt is when the item expired (zero unless expired).
	ExpiredAt time.Time
	// ResolvedAt is when the item was resolved (zero unless resolved).
	ResolvedAt time.Time
	// Flagged reports that the work needs an operator's attention: retrying
	// alone will not complete it.
	Flagged bool
	// FlagReason says why the item is flagged.
	FlagReason string
	// Payload is the caller's data, pinned by the first [ProducerStorage.Mark] and
	// replaced only by [ProducerStorage.Reopen].
	Payload P
}

// Attempt returns the key, generation and age observed in i for reporting a
// worker outcome to an [AttemptStorage].
func (i Item[P]) Attempt() Attempt {
	return Attempt{Key: i.Key, Generation: i.Generation, Since: i.Since}
}
