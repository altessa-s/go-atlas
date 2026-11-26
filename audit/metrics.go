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

const labelReason = "reason"

// Reason label values recorded on events_dropped_total.
const (
	// DropReasonNotStarted means [Auditor.Emit] ran before [Auditor.Start].
	DropReasonNotStarted = "not_started"
	// DropReasonShutdown means [Auditor.Emit] ran after [Auditor.Shutdown].
	DropReasonShutdown = "shutdown"
	// DropReasonDispatcherClosed means the underlying dispatcher was not started or already closed.
	DropReasonDispatcherClosed = "dispatcher_closed"
	// DropReasonQueueFull means the dispatcher's in-memory buffer had no capacity.
	DropReasonQueueFull = "queue_full"
	// DropReasonWALFull means the dispatcher's WAL reached its byte cap.
	DropReasonWALFull = "wal_full"
	// DropReasonRejected means the dispatcher rejected the event without a more specific reason.
	DropReasonRejected = "rejected"
)

// auditMetrics holds Prometheus metrics for the Auditor facade.
// Dispatch-level metrics (workers, flush duration, WAL) are provided
// by the underlying [dispatch.Engine] and are not duplicated here.
//
// When no [metrics.Meter] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type auditMetrics struct {
	eventsEmitted metrics.Counter
	eventsDropped metrics.Counter
}

func newAuditMetrics(c metrics.Meter, subsystem string) *auditMetrics {
	if c == nil {
		c = metrics.Noop()
	}
	if subsystem == "" {
		subsystem = DefaultMetricsSubsystem
	}

	scoped := metrics.MustMeter{Meter: c.WithSubsystem(subsystem)}

	return &auditMetrics{
		eventsEmitted: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_emitted_total",
			Help: "Total number of audit events successfully submitted to the dispatcher.",
		}),
		eventsDropped: scoped.MustCounter(metrics.MetricOpts{
			Name:       "events_dropped_total",
			Help:       "Total number of audit events dropped before dispatch or rejected by the dispatcher, partitioned by reason.",
			LabelNames: []string{labelReason},
		}),
	}
}
