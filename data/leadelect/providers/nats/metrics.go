// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// natsLeaderMetrics holds all Prometheus metrics for the NATS leader election provider.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type natsLeaderMetrics struct {
	leaseOperations   metrics.Counter
	isLeader          metrics.Gauge
	campingIterations metrics.Timer
}

func newNatsLeaderMetrics(c metrics.Collector) *natsLeaderMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("nats_leader_election")

	return &natsLeaderMetrics{
		leaseOperations: scoped.MustCounter(metrics.MetricOpts{
			Name:       "lease_operations_total",
			Help:       "Total number of lease operations.",
			LabelNames: []string{"op", "result"},
		}),
		isLeader: scoped.MustGauge(metrics.MetricOpts{
			Name: "is_leader",
			Help: "Whether this instance is the current leader (1=leader, 0=follower).",
		}),
		campingIterations: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "camping_iteration_duration_seconds",
				Help: "Duration of camping loop iterations in seconds.",
			},
		}),
	}
}
