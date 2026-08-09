// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// outboxMetrics holds all Prometheus metrics for the Outbox.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type outboxMetrics struct {
	eventsDispatched    metrics.Counter
	eventsDispatchFail  metrics.Counter
	eventsSaved         metrics.Counter
	eventsSkipped       metrics.Counter
	eventsRejected      metrics.Counter
	eventsInFlight      metrics.Gauge
	dispatchDuration    metrics.Timer
	unlockDuration      metrics.Timer
	cleanupDuration     metrics.Timer
	dispatchRetries     metrics.Counter
	maxRetriesExhausted metrics.Counter
	eventsExpired       metrics.Counter
	expireDuration      metrics.Timer

	// Backlog gauges, refreshed by the stats cycle. Counters alone report the
	// rate of failures but not the size of the pile they leave behind: an
	// outbox that stopped draining and an outbox with nothing to do both emit
	// zero. These are what a depth or lag alert can be written against.
	pendingEvents      metrics.Gauge
	inProgressEvents   metrics.Gauge
	deadLetteredEvents metrics.Gauge
	oldestPendingAge   metrics.Gauge
	statsDuration      metrics.Timer
}

func newOutboxMetrics(c metrics.Collector) *outboxMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("outbox")

	return &outboxMetrics{
		eventsDispatched: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_dispatched_total",
			Help: "Total number of events successfully dispatched.",
		}),
		eventsDispatchFail: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_dispatch_attempts_failed_total",
			Help: "Total number of failed delivery attempts.",
		}),
		eventsSaved: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_saved_total",
			Help: "Total number of events persisted to the outbox store.",
		}),
		eventsSkipped: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_skipped_total",
			Help: "Total number of events skipped due to key compaction.",
		}),
		eventsInFlight: scoped.MustGauge(metrics.MetricOpts{
			Name: "events_in_flight",
			Help: "Number of events currently being dispatched.",
		}),
		dispatchDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "dispatch_cycle_duration_seconds",
				Help: "Duration of a single dispatch cycle in seconds.",
			},
		}),
		unlockDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "unlock_cycle_duration_seconds",
				Help: "Duration of a single unlock cycle in seconds.",
			},
		}),
		cleanupDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "cleanup_cycle_duration_seconds",
				Help: "Duration of a single cleanup cycle in seconds.",
			},
		}),
		dispatchRetries: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_retries_scheduled_total",
			Help: "Total number of failed dispatches rescheduled for a later attempt.",
		}),
		maxRetriesExhausted: scoped.MustCounter(metrics.MetricOpts{
			Name: "max_retries_exhausted_total",
			Help: "Total number of events that exhausted all retry attempts.",
		}),
		eventsExpired: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_expired_total",
			Help: "Total number of events that expired before successful dispatch.",
		}),
		expireDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "expire_cycle_duration_seconds",
				Help: "Duration of a single expire cycle in seconds.",
			},
		}),
		eventsRejected: scoped.MustCounter(metrics.MetricOpts{
			Name: "events_rejected_total",
			Help: "Total number of events dead-lettered as permanently undeliverable.",
		}),
		pendingEvents: scoped.MustGauge(metrics.MetricOpts{
			Name: "events_pending",
			Help: "Number of events awaiting dispatch (pending plus failed).",
		}),
		inProgressEvents: scoped.MustGauge(metrics.MetricOpts{
			Name: "events_in_progress",
			Help: "Number of events currently locked by a dispatcher.",
		}),
		deadLetteredEvents: scoped.MustGauge(metrics.MetricOpts{
			Name: "events_dead_lettered",
			Help: "Dead-letter depth: events that exhausted their retries or were rejected.",
		}),
		oldestPendingAge: scoped.MustGauge(metrics.MetricOpts{
			Name: "events_oldest_pending_age_seconds",
			Help: "Age of the oldest undispatched event in seconds — the outbox equivalent of consumer lag.",
		}),
		statsDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "stats_cycle_duration_seconds",
				Help: "Duration of a single stats cycle in seconds.",
			},
		}),
	}
}
