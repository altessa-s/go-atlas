// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// DefaultMetricsSubsystem is the Prometheus subsystem name used when no
// explicit subsystem is provided via [WithMetricsSubsystem].
const DefaultMetricsSubsystem = "audit"

// auditMetrics holds Prometheus metrics for the Auditor facade.
// Dispatch-level metrics (workers, flush duration, WAL) are provided
// by the underlying [dispatch.Engine] and are not duplicated here.
//
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type auditMetrics struct {
	eventsEmitted metrics.Counter
	eventsDropped metrics.Counter
}

func newAuditMetrics(c metrics.Collector, subsystem string) *auditMetrics {
	if c == nil {
		c = metrics.Noop()
	}
	if subsystem == "" {
		subsystem = DefaultMetricsSubsystem
	}

	scoped := c.WithSubsystem(subsystem)

	return &auditMetrics{
		eventsEmitted: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_emitted_total",
			Help: "Total number of audit events successfully submitted to the dispatcher.",
		}),
		eventsDropped: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_dropped_total",
			Help: "Total number of audit events dropped due to a full buffer.",
		}),
	}
}
