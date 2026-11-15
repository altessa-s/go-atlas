// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/ext/core/dispatch/observe"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

func TestObserver_RecordsEvents(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	o := observe.New(tc)

	o.ItemEnqueued()
	o.ItemEnqueued()
	o.ItemDropped()
	o.ItemsReplayed(3)
	o.EncodeFailed()
	o.WALAppendFailed()
	o.BatchStoreFailed(5)
	o.BatchDone(time.Millisecond)
	o.WorkerStarted()
	o.WorkerStarted()
	o.WorkerStopped()
	o.WALSize(1024)

	require.InDelta(t, 2, metricstest.GetCounterValue(t, tc, "test_async_items_enqueued_total"), 0)
	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_async_items_dropped_total"), 0)
	require.InDelta(t, 3, metricstest.GetCounterValue(t, tc, "test_async_wal_replay_total"), 0)
	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_async_encode_errors_total"), 0)
	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_async_wal_errors_total"), 0)
	require.InDelta(t, 5, metricstest.GetCounterValue(t, tc, "test_async_sink_errors_total"), 0)
	require.EqualValues(t, 1, metricstest.GetHistogramCount(t, tc, "test_async_batch_flush_duration_seconds"))
	require.InDelta(t, 1, metricstest.GetGaugeValue(t, tc, "test_async_workers_active"), 0)
	require.InDelta(t, 1024, metricstest.GetGaugeValue(t, tc, "test_async_wal_bytes"), 0)
}

func TestObserver_MetricsSubsystem(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	observe.New(tc, observe.WithMetricsSubsystem("audit")).ItemEnqueued()

	require.InDelta(t, 1, metricstest.GetCounterValue(t, tc, "test_audit_items_enqueued_total"), 0)
}

func TestObserver_NilMeter(t *testing.T) {
	t.Parallel()

	o := observe.New(nil)
	require.NotPanics(t, func() {
		o.ItemEnqueued()
		o.BatchDone(time.Second)
		o.WALSize(1)
	})
}
