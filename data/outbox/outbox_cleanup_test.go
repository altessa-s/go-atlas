// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type cleanupCallStore struct {
	deleteCalls atomic.Int64
}

func (s *cleanupCallStore) FetchUnprocessedEvents(ctx context.Context, batchSize uint32, lastAttemptBefore time.Time) ([]Event, error) {
	return nil, nil
}
func (s *cleanupCallStore) DeleteProcessedEvents(ctx context.Context, since time.Time) error {
	s.deleteCalls.Add(1)
	return nil
}
func (s *cleanupCallStore) UnlockStuckEvents(ctx context.Context, lockedBefore time.Time) error {
	return nil
}
func (s *cleanupCallStore) SaveEvents(ctx context.Context, events ...Event) error   { return nil }
func (s *cleanupCallStore) UpdateEvents(ctx context.Context, events ...Event) error { return nil }

func noopHandler(_ context.Context, _ Event) error { return nil }

func TestOutbox_CleanupDisabledWhenLifetimeIsNonPositive(t *testing.T) {
	store := &cleanupCallStore{}

	o := New(
		store,
		noopHandler,
		// publishedEventsLifetime remains default (-1) => cleanup disabled
	)

	if err := o.RunCleanupCycle(t.Context()); err != nil {
		t.Fatalf("RunCleanupCycle failed: %v", err)
	}

	if got := store.deleteCalls.Load(); got != 0 {
		t.Fatalf("DeleteProcessedEvents calls=%d, want 0 when lifetime<=0", got)
	}
}
