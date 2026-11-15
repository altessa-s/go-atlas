// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natsprovider

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// labelSubject is the metric label holding the subscription subject.
const labelSubject = "subject"

// subscriberMetrics holds all Prometheus metrics for the NATS subscriber.
// When no [metrics.Meter] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type subscriberMetrics struct {
	messagesReceived   metrics.Counter
	processingDuration metrics.Timer
	processingErrors   metrics.Counter
}

func newSubscriberMetrics(c metrics.Meter) *subscriberMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("broker")

	return &subscriberMetrics{
		messagesReceived: scoped.MustCounter(metrics.MetricOpts{
			Name:       "messages_received_total",
			Help:       "Total number of messages received by subscribers.",
			LabelNames: []string{labelSubject},
		}),
		processingDuration: scoped.MustTimer(metrics.HistogramOpts{
			Name:       "message_processing_duration_seconds",
			Help:       "Duration of message handler execution in seconds.",
			LabelNames: []string{labelSubject},
		}),
		processingErrors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "message_processing_errors_total",
			Help:       "Total number of message handler failures.",
			LabelNames: []string{labelSubject},
		}),
	}
}
