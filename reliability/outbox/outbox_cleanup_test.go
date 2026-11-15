// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cleanupCallStore struct {
	deleteCalls atomic.Int64
}

func (s *cleanupCallStore) FetchUnprocessedEvents(ctx context.Context, batchSize uint32) ([]Event, error) {
	return nil, nil
}
func (s *cleanupCallStore) DeleteProcessedEvents(ctx context.Context, olderThan time.Duration) error {
	s.deleteCalls.Add(1)
	return nil
}
func (s *cleanupCallStore) UnlockStuckEvents(ctx context.Context, lockExpiry time.Duration) error {
	return nil
}
func (s *cleanupCallStore) SaveEvents(ctx context.Context, events ...Event) error   { return nil }
func (s *cleanupCallStore) UpdateEvents(ctx context.Context, events ...Event) error { return nil }
func (s *cleanupCallStore) ExpireEvents(ctx context.Context) (int64, error) {
	return 0, nil
}
func (s *cleanupCallStore) Stats(ctx context.Context) (Stats, error) { return Stats{}, nil }

func noopHandler(_ context.Context, _ Event) error { return nil }

func TestOutbox_CleanupDisabledWhenLifetimeIsNonPositive(t *testing.T) {
	store := &cleanupCallStore{}

	o := New(
		store,
		noopHandler,
		// publishedEventsLifetime remains default (-1) => cleanup disabled
	)

	require.NoError(t, o.RunCleanupCycle(t.Context()))
	require.Equal(t, int64(0), store.deleteCalls.Load())
}
