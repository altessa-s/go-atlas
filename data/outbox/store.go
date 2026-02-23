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
type Store interface {
	// FetchUnprocessedEvents retrieves pending or failed events eligible for retry.
	// Implementations should lock fetched events to prevent concurrent processing.
	FetchUnprocessedEvents(ctx context.Context, batchSize uint32, lastAttemptBefore time.Time) ([]Event, error)

	// DeleteProcessedEvents removes processed events (sent or skipped) older than since.
	DeleteProcessedEvents(ctx context.Context, since time.Time) error

	// UnlockStuckEvents unlocks events locked before the specified time.
	UnlockStuckEvents(ctx context.Context, lockedBefore time.Time) error

	// SaveEvents persists new events (typically StatusPending) to the store.
	SaveEvents(ctx context.Context, events ...Event) error

	// UpdateEvents updates existing event state after processing attempts.
	UpdateEvents(ctx context.Context, events ...Event) error
}
