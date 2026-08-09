// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

type statsStore struct {
	recordingStore
	stats      Stats
	err        error
	statsCalls atomic.Int64
}

func (s *statsStore) Stats(context.Context) (Stats, error) {
	s.statsCalls.Add(1)
	return s.stats, s.err
}

// The gauges are the only thing an operator can alert on: a stalled outbox and
// an idle one emit identical counters, and differ only in backlog depth.
func TestRunStatsCycle_PublishesBacklogGauges(t *testing.T) {
	t.Parallel()

	tc := testhelpers.NewTestCollector()
	store := &statsStore{stats: Stats{
		Pending:          7,
		InProgress:       2,
		DeadLettered:     3,
		OldestPendingAge: 90 * time.Second,
	}}

	ob := New(store, noopHandler, WithCollector(tc))
	require.NoError(t, ob.RunStatsCycle(t.Context()))

	require.InDelta(t, 7, testhelpers.GetGaugeValue(t, tc, "test_outbox_events_pending"), 0.001)
	require.InDelta(t, 2, testhelpers.GetGaugeValue(t, tc, "test_outbox_events_in_progress"), 0.001)
	require.InDelta(t, 3, testhelpers.GetGaugeValue(t, tc, "test_outbox_events_dead_lettered"), 0.001)
	require.InDelta(t, 90, testhelpers.GetGaugeValue(t, tc, "test_outbox_events_oldest_pending_age_seconds"), 0.001)
}

func TestRunStatsCycle_PropagatesStoreError(t *testing.T) {
	t.Parallel()

	storeErr := errors.New("mongo unavailable")
	ob := New(&statsStore{err: storeErr}, noopHandler)

	require.ErrorIs(t, ob.RunStatsCycle(t.Context()), storeErr)
}

func TestRunStatsCycle_SchedulerManaged(t *testing.T) {
	t.Parallel()

	ob := New(&statsStore{}, noopHandler)
	_ = ob.RegisterStatsSchedulerFunc()

	require.ErrorIs(t, ob.RunStatsCycle(t.Context()), corescheduler.ErrSchedulerManaged)
}
