// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"time"
)

// Store defines the interface for persistent outbox event storage.
// Implementations manage event lifecycle: save, retrieve, update, and cleanup.
//
// Time-window arguments are passed as durations (policy), never as absolute
// client timestamps. Implementations MUST evaluate the corresponding deadline
// against their own backend clock (e.g. MongoDB's $$NOW on the primary) so that
// instances with skewed wall clocks cannot disagree on whether an event is due,
// stuck, or expired. See [Store.FetchUnprocessedEvents] and [Store.UnlockStuckEvents].
type Store interface {
	// FetchUnprocessedEvents retrieves pending events, and failed events whose
	// last attempt is older than retryAfter, then locks them for processing.
	// The retry deadline (now-retryAfter) and the lock timestamp are evaluated
	// against the backend clock, not the caller's, to stay clock-skew safe.
	FetchUnprocessedEvents(ctx context.Context, batchSize uint32, retryAfter time.Duration) ([]Event, error)

	// DeleteProcessedEvents removes processed events (sent, skipped, or expired)
	// whose publication is older than olderThan, evaluated against the backend clock.
	DeleteProcessedEvents(ctx context.Context, olderThan time.Duration) error

	// UnlockStuckEvents resets in-progress events locked for longer than lockExpiry
	// back to pending, with the lock age evaluated against the backend clock.
	UnlockStuckEvents(ctx context.Context, lockExpiry time.Duration) error

	// SaveEvents persists new events (typically StatusPending) to the store.
	SaveEvents(ctx context.Context, events ...Event) error

	// UpdateEvents updates existing event state after processing attempts.
	UpdateEvents(ctx context.Context, events ...Event) error

	// ExpireEvents marks pending or failed events whose ExpiresAt has passed as
	// expired, with the deadline evaluated against the backend clock.
	// Returns the number of events expired.
	ExpireEvents(ctx context.Context) (int64, error)
}
