// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// leaderMetrics holds all Prometheus metrics for the leader election coordinator.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type leaderMetrics struct {
	transitions      metrics.Counter
	isLeader         metrics.Gauge
	callbackDuration metrics.Timer
	callbackErrors   metrics.Counter
}

func newLeaderMetrics(c metrics.Collector) *leaderMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("leader_election")

	return &leaderMetrics{
		transitions: scoped.MustCounter(metrics.MetricOpts{
			Name:       "transitions_total",
			Help:       "Total number of leadership transitions.",
			LabelNames: []string{"type"},
		}),
		isLeader: scoped.MustGauge(metrics.MetricOpts{
			Name: "is_leader",
			Help: "Whether this instance is the current leader (1=leader, 0=follower).",
		}),
		callbackDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "callback_duration_seconds",
				Help: "Duration of leadership callback executions in seconds.",
			},
		}),
		callbackErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "callback_errors_total",
			Help: "Total number of failed leadership callback executions.",
		}),
	}
}
