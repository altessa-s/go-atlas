// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/service/scheduler"
	"github.com/altessa-s/go-atlas/service/scheduler/storages/memory"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	promadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
	promio "github.com/prometheus/client_model/go"
)

func TestScheduler_Metrics_Noop(t *testing.T) {
	storage := memory.New(100)
	s := scheduler.New(storage, scheduler.WithTickInterval(50*time.Millisecond))

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32
	err := s.Register(ctx, corescheduler.TaskConfig{
		ID:         "noop-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	})
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, execCount.Load(), int32(1), "task should have executed at least once")
}

func TestScheduler_Metrics_TasksRegistered(t *testing.T) {
	registry := prometheus.NewRegistry()
	adapter := promadapter.New(promadapter.WithRegistry(registry))
	collector := metrics.New(metrics.WithServiceName("test"), metrics.WithAdapter(adapter))

	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithCollector(collector),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	noop := func(_ context.Context) error { return nil }

	// Register two tasks
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID: "task-a", Schedule: "@every 1h", Func: noop,
	}))
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID: "task-b", Schedule: "@every 1h", Func: noop,
	}))

	val := getGaugeValue(t, registry, "test_scheduler_tasks_registered")
	assert.Equal(t, float64(2), val, "should have 2 registered tasks")

	// Unregister one
	require.NoError(t, s.Unregister(ctx, "task-a"))

	val = getGaugeValue(t, registry, "test_scheduler_tasks_registered")
	assert.Equal(t, float64(1), val, "should have 1 registered task after unregister")
}

func TestScheduler_Metrics_TaskExecution(t *testing.T) {
	registry := prometheus.NewRegistry()
	adapter := promadapter.New(promadapter.WithRegistry(registry))
	collector := metrics.New(metrics.WithServiceName("test"), metrics.WithAdapter(adapter))

	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithCollector(collector),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         "metrics-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Priority:   corescheduler.TaskPriorityNormal,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return nil
		},
	}))

	// Wait for at least one execution
	require.Eventually(t, func() bool { return execCount.Load() >= 1 },
		2*time.Second, 50*time.Millisecond)

	// Check tasksDispatched counter
	dispatched := getCounterValue(t, registry, "test_scheduler_tasks_dispatched_total",
		"task_id", "metrics-task", "priority", "normal")
	assert.GreaterOrEqual(t, dispatched, float64(1), "tasks_dispatched_total should be >= 1")

	// Check taskDuration histogram has observations
	duration := getHistogramCount(t, registry, "test_scheduler_task_duration_seconds",
		"task_id", "metrics-task", "priority", "normal")
	assert.GreaterOrEqual(t, duration, uint64(1), "task_duration_seconds should have >= 1 observation")

	// Check tickDuration has observations
	tickCount := getHistogramCount(t, registry, "test_scheduler_tick_duration_seconds")
	assert.GreaterOrEqual(t, tickCount, uint64(1), "tick_duration_seconds should have >= 1 observation")
}

func TestScheduler_Metrics_TaskErrors(t *testing.T) {
	registry := prometheus.NewRegistry()
	adapter := promadapter.New(promadapter.WithRegistry(registry))
	collector := metrics.New(metrics.WithServiceName("test"), metrics.WithAdapter(adapter))

	storage := memory.New(100)
	s := scheduler.New(storage,
		scheduler.WithTickInterval(50*time.Millisecond),
		scheduler.WithCollector(collector),
	)

	ctx := t.Context()
	require.NoError(t, s.Start(ctx))
	defer func() {
		stopCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_ = s.Stop(stopCtx)
	}()

	var execCount atomic.Int32
	require.NoError(t, s.Register(ctx, corescheduler.TaskConfig{
		ID:         "failing-task",
		Schedule:   "@every 1s",
		RunOnStart: true,
		Func: func(_ context.Context) error {
			execCount.Add(1)
			return errors.New("task failed")
		},
	}))

	require.Eventually(t, func() bool { return execCount.Load() >= 1 },
		2*time.Second, 50*time.Millisecond)

	errorCount := getCounterValue(t, registry, "test_scheduler_task_errors_total",
		"task_id", "failing-task")
	assert.GreaterOrEqual(t, errorCount, float64(1), "task_errors_total should be >= 1")
}

// getGaugeValue finds a gauge metric by name and returns its value.
func getGaugeValue(t *testing.T, registry *prometheus.Registry, name string, labelPairs ...string) float64 {
	t.Helper()
	mf := gatherMetric(t, registry, name)
	if mf == nil {
		return 0
	}
	m := findMetricByLabels(mf.GetMetric(), labelPairs...)
	if m == nil {
		return 0
	}
	return m.GetGauge().GetValue()
}

// getCounterValue finds a counter metric by name and label pairs, returning its value.
func getCounterValue(t *testing.T, registry *prometheus.Registry, name string, labelPairs ...string) float64 {
	t.Helper()
	mf := gatherMetric(t, registry, name)
	if mf == nil {
		return 0
	}
	m := findMetricByLabels(mf.GetMetric(), labelPairs...)
	if m == nil {
		return 0
	}
	return m.GetCounter().GetValue()
}

// getHistogramCount finds a histogram metric by name and returns its sample count.
func getHistogramCount(t *testing.T, registry *prometheus.Registry, name string, labelPairs ...string) uint64 {
	t.Helper()
	mf := gatherMetric(t, registry, name)
	if mf == nil {
		return 0
	}
	m := findMetricByLabels(mf.GetMetric(), labelPairs...)
	if m == nil {
		return 0
	}
	return m.GetHistogram().GetSampleCount()
}

// gatherMetric gathers all metrics from the registry and returns the named family.
func gatherMetric(t *testing.T, registry *prometheus.Registry, name string) *promio.MetricFamily {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, mf := range families {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

// findMetricByLabels finds a metric within a family matching the given label key-value pairs.
func findMetricByLabels(ms []*promio.Metric, labelPairs ...string) *promio.Metric {
	if len(labelPairs) == 0 {
		if len(ms) > 0 {
			return ms[0]
		}
		return nil
	}
	for _, m := range ms {
		if matchLabels(m.GetLabel(), labelPairs...) {
			return m
		}
	}
	return nil
}

// matchLabels checks whether a metric's labels match all given key-value pairs.
func matchLabels(labels []*promio.LabelPair, pairs ...string) bool {
	for i := 0; i < len(pairs)-1; i += 2 {
		key, val := pairs[i], pairs[i+1]
		found := false
		for _, lp := range labels {
			if lp.GetName() == key && lp.GetValue() == val {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
