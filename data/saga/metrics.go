// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// sagaMetrics holds all Prometheus metrics for the orchestrator. When no
// [metrics.Collector] is provided, [metrics.Noop] is used and every method
// becomes a zero-cost no-op.
type sagaMetrics struct {
	started     metrics.Counter
	completed   metrics.Counter
	compensated metrics.Counter
	failed      metrics.Counter
	inFlight    metrics.Gauge

	stepsExecuted metrics.Counter
	stepFailures  metrics.Counter
	stepRetries   metrics.Counter
	compensations metrics.Counter
	compFailures  metrics.Counter

	recoveryCycles metrics.Counter
	recovered      metrics.Counter

	stageDuration metrics.Timer
}

func newSagaMetrics(c metrics.Collector) *sagaMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("saga")

	return &sagaMetrics{
		started: scoped.MustCounter(metrics.MetricOpts{
			Name: "started_total",
			Help: "Total number of saga instances started.",
		}),
		completed: scoped.MustCounter(metrics.MetricOpts{
			Name: "completed_total",
			Help: "Total number of saga instances that committed all stages.",
		}),
		compensated: scoped.MustCounter(metrics.MetricOpts{
			Name: "compensated_total",
			Help: "Total number of saga instances that fully rolled back.",
		}),
		failed: scoped.MustCounter(metrics.MetricOpts{
			Name: "failed_total",
			Help: "Total number of saga instances that entered the unrecoverable Failed state.",
		}),
		inFlight: scoped.MustGauge(metrics.MetricOpts{
			Name: "in_flight",
			Help: "Number of saga instances currently executing.",
		}),
		stepsExecuted: scoped.MustCounter(metrics.MetricOpts{
			Name: "steps_executed_total",
			Help: "Total number of forward step actions that committed.",
		}),
		stepFailures: scoped.MustCounter(metrics.MetricOpts{
			Name: "step_failures_total",
			Help: "Total number of forward step actions that failed after retries.",
		}),
		stepRetries: scoped.MustCounter(metrics.MetricOpts{
			Name: "step_retries_total",
			Help: "Total number of step action retry attempts.",
		}),
		compensations: scoped.MustCounter(metrics.MetricOpts{
			Name: "compensations_total",
			Help: "Total number of compensations that ran successfully.",
		}),
		compFailures: scoped.MustCounter(metrics.MetricOpts{
			Name: "compensation_failures_total",
			Help: "Total number of compensations that failed after retries.",
		}),
		recoveryCycles: scoped.MustCounter(metrics.MetricOpts{
			Name: "recovery_cycles_total",
			Help: "Total number of background recovery cycles executed.",
		}),
		recovered: scoped.MustCounter(metrics.MetricOpts{
			Name: "recovered_total",
			Help: "Total number of stalled or timed-out instances picked up by recovery.",
		}),
		stageDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "stage_duration_seconds",
				Help: "Duration of forward stage execution in seconds.",
			},
		}),
	}
}
