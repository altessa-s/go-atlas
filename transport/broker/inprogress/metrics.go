// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package inprogress

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// inprogressMetrics holds all Prometheus metrics for the InProgress heartbeat manager.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type inprogressMetrics struct {
	heartbeatsSent  metrics.Counter
	heartbeatErrors metrics.Counter
}

func newInprogressMetrics(c metrics.Collector) *inprogressMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("broker_inprogress")

	return &inprogressMetrics{
		heartbeatsSent: scoped.MustCounter(metrics.MetricOpts{
			Name: "heartbeats_sent_total",
			Help: "Total number of InProgress heartbeats sent.",
		}),
		heartbeatErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "heartbeat_errors_total",
			Help: "Total number of failed InProgress heartbeat attempts.",
		}),
	}
}
