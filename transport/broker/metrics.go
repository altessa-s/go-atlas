// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package broker

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// brokerMetrics holds all Prometheus metrics for the Broker.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type brokerMetrics struct {
	messagesPublished metrics.Counter
	publishErrors     metrics.Counter
	publishDuration   metrics.Timer
}

func newBrokerMetrics(c metrics.Collector) *brokerMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("broker")

	return &brokerMetrics{
		messagesPublished: scoped.MustCounter(metrics.MetricOpts{
			Name: "messages_published_total",
			Help: "Total number of messages successfully published.",
		}),
		publishErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "publish_errors_total",
			Help: "Total number of message publish failures.",
		}),
		publishDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "publish_duration_seconds",
				Help: "Duration of publish operations in seconds.",
			},
		}),
	}
}
