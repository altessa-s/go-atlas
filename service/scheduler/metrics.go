// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// schedulerMetrics holds all Prometheus metrics for the scheduler.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
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
}

func newSchedulerMetrics(c metrics.Collector) *schedulerMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("scheduler")

	return &schedulerMetrics{
		tasksDispatched: scoped.MustCounter(metrics.MetricOpts{
			Name:       "tasks_dispatched_total",
			Help:       "Total number of task executions started.",
			LabelNames: []string{"task_id", "priority"},
		}),
		taskDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "task_duration_seconds",
				Help:       "Duration of task executions in seconds.",
				LabelNames: []string{"task_id", "priority"},
			},
		}),
		taskErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "task_errors_total",
			Help:       "Total number of failed task executions.",
			LabelNames: []string{"task_id"},
		}),
		tasksSkipped: scoped.MustCounter(metrics.MetricOpts{
			Name:       "tasks_skipped_total",
			Help:       "Total number of task executions skipped via SkipNextRun.",
			LabelNames: []string{"task_id"},
		}),
		staleTasksRecovered: scoped.MustCounter(metrics.MetricOpts{
			Name: "stale_tasks_recovered_total",
			Help: "Total number of stale task recoveries.",
		}),
		tickDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "tick_duration_seconds",
				Help: "Duration of each scheduler tick cycle in seconds.",
			},
		}),
		tasksRunning: scoped.MustGauge(metrics.MetricOpts{
			Name: "tasks_running",
			Help: "Number of currently executing tasks.",
		}),
		tasksRegistered: scoped.MustGauge(metrics.MetricOpts{
			Name: "tasks_registered",
			Help: "Total number of registered tasks.",
		}),
	}
}
