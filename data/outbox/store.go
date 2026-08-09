// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"time"
)

// Stats is a point-in-time snapshot of the outbox backlog, used to publish the
// gauges an operator alerts on: queue depth, dead-letter depth, and dispatch lag.
// A dead-lettered event is never retried and never cleaned up, so without these
// numbers a stuck outbox is indistinguishable from an idle one.
type Stats struct {
	// Pending counts events awaiting dispatch — StatusPending plus every
	// StatusFailed event, whether or not its backoff has elapsed.
	Pending int64

	// InProgress counts events currently locked by a dispatcher.
	InProgress int64

	// DeadLettered counts terminally undeliverable events
	// (StatusMaxAttemptReached and StatusRejected). This is the DLQ depth.
	DeadLettered int64

	// OldestPendingAge is how long the oldest undispatched event has been
	// waiting, measured against the backend clock. Zero when nothing is
	// pending. This is the outbox equivalent of consumer lag.
	OldestPendingAge time.Duration
}

// Store defines the interface for persistent outbox event storage.
// Implementations manage event lifecycle: save, retrieve, update, and cleanup.
//
// Time-window arguments are passed as durations (policy), never as absolute
// client timestamps. Implementations MUST evaluate the corresponding deadline
// against their own backend clock (e.g. MongoDB's $$NOW on the primary) so that
// instances with skewed wall clocks cannot disagree on whether an event is due,
// stuck, or expired. See [Store.FetchUnprocessedEvents] and [Store.UnlockStuckEvents].
type Store interface {
	// FetchUnprocessedEvents retrieves events that are ready to dispatch —
	// pending events, plus failed events whose [Event.RetryAfter] backoff has
	// elapsed — then locks them for processing.
	//
	// Implementations MUST return the batch sorted by [Event.CreatedAt]
	// ascending (oldest first): key compaction treats the last event of each
	// key group as the newest one, so an unsorted batch would dispatch a stale
	// event and discard the current one.
	//
	// Each returned event MUST carry a [Event.LockToken] identifying the lock
	// this call took, so [Store.UpdateEvents] can fence stale writes.
	FetchUnprocessedEvents(ctx context.Context, batchSize uint32) ([]Event, error)

	// DeleteProcessedEvents removes processed events (sent, skipped, or expired)
	// whose publication is older than olderThan, evaluated against the backend clock.
	// Terminally failed events (max-attempt-reached, rejected) are deliberately
	// retained: they are the dead-letter queue and must survive for inspection.
	DeleteProcessedEvents(ctx context.Context, olderThan time.Duration) error

	// UnlockStuckEvents resets in-progress events locked for longer than lockExpiry
	// back to pending, with the lock age evaluated against the backend clock.
	// Unlocking MUST invalidate the event's lock token.
	UnlockStuckEvents(ctx context.Context, lockExpiry time.Duration) error

	// SaveEvents persists new events (typically StatusPending) to the store.
	SaveEvents(ctx context.Context, events ...Event) error

	// UpdateEvents updates existing event state after processing attempts.
	//
	// A write MUST be applied only while the stored lock token still equals the
	// event's [Event.LockToken]; otherwise the lock was revoked and reassigned,
	// and applying the write would clobber a newer attempt's result.
	// [Event.RetryAfter] MUST be anchored to the backend clock, never to a
	// timestamp computed by the caller.
	UpdateEvents(ctx context.Context, events ...Event) error

	// ExpireEvents marks pending or failed events whose ExpiresAt has passed as
	// expired, with the deadline evaluated against the backend clock.
	// Returns the number of events expired.
	ExpireEvents(ctx context.Context) (int64, error)

	// Stats returns a snapshot of the backlog for the outbox gauges.
	Stats(ctx context.Context) (Stats, error)
}
