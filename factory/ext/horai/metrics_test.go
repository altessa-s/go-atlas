// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	schedtask "github.com/altessa-s/go-horai/task"
)

func TestMetrics_ReportsSchedulerObservations(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	m := newMetrics(tc)

	m.TaskDispatched("job", schedtask.PriorityNormal)
	m.TaskFinished("job", schedtask.PriorityNormal, time.Second, errors.New("failed"))
	m.TaskFinished("job", schedtask.PriorityNormal, time.Second, nil)
	m.TaskSkipped("job")
	m.StaleRecovered()
	m.StorageError("claim_run")
	m.RunningChanged(3)
	m.RegisteredChanged(5)
	m.TickDone(time.Millisecond)
	m.DispatchLag("job", time.Millisecond)

	const prefix = "test_scheduler_"
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, prefix+"tasks_dispatched_total", "task_id", "job", "priority", "normal"))
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, prefix+"task_errors_total", "task_id", "job"))
	require.Equal(t, uint64(2), metricstest.GetHistogramCount(t, tc, prefix+"task_duration_seconds", "task_id", "job", "priority", "normal"))
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, prefix+"tasks_skipped_total", "task_id", "job"))
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, prefix+"stale_tasks_recovered_total"))
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, tc, prefix+"storage_errors_total", "op", "claim_run"))
	require.Equal(t, 3.0, metricstest.GetGaugeValue(t, tc, prefix+"tasks_running"))
	require.Equal(t, 5.0, metricstest.GetGaugeValue(t, tc, prefix+"tasks_registered"))
}
