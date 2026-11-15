// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"time"

	"github.com/altessa-s/go-atlas/observability/metrics"

	horai "github.com/altessa-s/go-horai"
	schedtask "github.com/altessa-s/go-horai/task"
)

// Metric label names.
const (
	labelTaskID   = "task_id"
	labelPriority = "priority"
)

// schedulerMetrics reports the go-horai scheduler observations ([horai.Metrics])
// as go-atlas metrics.
// When no [metrics.Meter] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type schedulerMetrics struct {
	tasksDispatched     metrics.Counter
	taskDuration        metrics.Timer
	taskErrors          metrics.Counter
	tasksSkipped        metrics.Counter
	staleTasksRecovered metrics.Counter
	tickDuration        metrics.Timer
	tasksRunning        metrics.Gauge
	tasksRegistered     metrics.Gauge
	dispatchLag         metrics.Timer
	storageErrors       metrics.Counter
}

func newMetrics(c metrics.Meter) *schedulerMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("scheduler")

	return &schedulerMetrics{
		tasksDispatched: scoped.MustCounter(metrics.MetricOpts{
			Name:       "tasks_dispatched_total",
			Help:       "Total number of task executions started.",
			LabelNames: []string{labelTaskID, labelPriority},
		}),
		taskDuration: scoped.MustTimer(metrics.HistogramOpts{
			Name:       "task_duration_seconds",
			Help:       "Duration of task executions in seconds.",
			LabelNames: []string{labelTaskID, labelPriority},
		}),
		taskErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "task_errors_total",
			Help:       "Total number of failed task executions.",
			LabelNames: []string{labelTaskID},
		}),
		tasksSkipped: scoped.MustCounter(metrics.MetricOpts{
			Name:       "tasks_skipped_total",
			Help:       "Total number of task executions skipped via SkipNextRun.",
			LabelNames: []string{labelTaskID},
		}),
		staleTasksRecovered: scoped.MustCounter(metrics.MetricOpts{
			Name: "stale_tasks_recovered_total",
			Help: "Total number of stale task recoveries.",
		}),
		tickDuration: scoped.MustTimer(metrics.HistogramOpts{
			Name: "tick_duration_seconds",
			Help: "Duration of each scheduler tick cycle in seconds.",
		}),
		tasksRunning: scoped.MustGauge(metrics.MetricOpts{
			Name: "tasks_running",
			Help: "Number of currently executing tasks.",
		}),
		tasksRegistered: scoped.MustGauge(metrics.MetricOpts{
			Name: "tasks_registered",
			Help: "Total number of registered tasks.",
		}),
		dispatchLag: scoped.MustTimer(metrics.HistogramOpts{
			Name:       "dispatch_lag_seconds",
			Help:       "Delay between scheduled and actual task execution in seconds.",
			LabelNames: []string{labelTaskID},
		}),
		storageErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "storage_errors_total",
			Help:       "Total number of scheduler storage operation failures.",
			LabelNames: []string{"op"},
		}),
	}
}

var _ horai.Metrics = (*schedulerMetrics)(nil)

// TaskDispatched counts a started execution.
func (m *schedulerMetrics) TaskDispatched(id string, p schedtask.Priority) {
	m.tasksDispatched.WithLabels(metrics.Labels{labelTaskID: id, labelPriority: p.String()}).Inc()
}

// TaskFinished records the duration of an execution and counts its failure.
func (m *schedulerMetrics) TaskFinished(id string, p schedtask.Priority, d time.Duration, err error) {
	m.taskDuration.WithLabels(metrics.Labels{labelTaskID: id, labelPriority: p.String()}).ObserveDuration(d)
	if err != nil {
		m.taskErrors.WithLabels(metrics.Labels{labelTaskID: id}).Inc()
	}
}

// TaskSkipped counts an execution skipped via SkipNextRun.
func (m *schedulerMetrics) TaskSkipped(id string) {
	m.tasksSkipped.WithLabels(metrics.Labels{labelTaskID: id}).Inc()
}

// DispatchLag records the delay between the scheduled and the actual start.
func (m *schedulerMetrics) DispatchLag(id string, d time.Duration) {
	m.dispatchLag.WithLabels(metrics.Labels{labelTaskID: id}).ObserveDuration(d)
}

// StaleRecovered counts a stale run recovery.
func (m *schedulerMetrics) StaleRecovered() { m.staleTasksRecovered.Inc() }

// TickDone records the duration of a tick.
func (m *schedulerMetrics) TickDone(d time.Duration) { m.tickDuration.ObserveDuration(d) }

// RunningChanged sets the number of executing tasks.
func (m *schedulerMetrics) RunningChanged(n int) { m.tasksRunning.Set(float64(n)) }

// RegisteredChanged sets the number of registered tasks.
func (m *schedulerMetrics) RegisteredChanged(n int) { m.tasksRegistered.Set(float64(n)) }

// StorageError counts a failed storage operation.
func (m *schedulerMetrics) StorageError(op string) {
	m.storageErrors.WithLabels(metrics.Labels{"op": op}).Inc()
}
