// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jetstream

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// engineMetrics holds the engine's Prometheus metrics. When no
// [metrics.Collector] is provided, [metrics.Noop] is used and every method
// becomes a zero-cost no-op.
type engineMetrics struct {
	submitted  metrics.Counter
	consumed   metrics.Counter
	acked      metrics.Counter
	naked      metrics.Counter
	terminated metrics.Counter
	ackErrors  metrics.Counter
	inFlight   metrics.Gauge
}

func newEngineMetrics(c metrics.Collector) *engineMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("saga_engine")

	return &engineMetrics{
		submitted: scoped.MustCounter(metrics.MetricOpts{
			Name: "submitted_total",
			Help: "Total number of saga start commands published to JetStream.",
		}),
		consumed: scoped.MustCounter(metrics.MetricOpts{
			Name: "consumed_total",
			Help: "Total number of saga start commands received from JetStream.",
		}),
		acked: scoped.MustCounter(metrics.MetricOpts{
			Name: "acked_total",
			Help: "Total number of commands acknowledged after their saga reached a terminal state.",
		}),
		naked: scoped.MustCounter(metrics.MetricOpts{
			Name: "naked_total",
			Help: "Total number of commands returned for redelivery after an interrupted execution.",
		}),
		terminated: scoped.MustCounter(metrics.MetricOpts{
			Name: "terminated_total",
			Help: "Total number of poison commands terminated without redelivery.",
		}),
		ackErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "ack_errors_total",
			Help: "Total number of failed Ack, Nak, Term or InProgress calls.",
		}),
		inFlight: scoped.MustGauge(metrics.MetricOpts{
			Name: "in_flight",
			Help: "Number of sagas the engine is currently driving.",
		}),
	}
}
