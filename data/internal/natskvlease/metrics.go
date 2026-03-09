// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package natskvlease

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// leaseMetrics holds all Prometheus metrics for NATS KV lease operations.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type leaseMetrics struct {
	operations metrics.Counter
	leaseHeld  metrics.Gauge
}

func newLeaseMetrics(c metrics.Collector) *leaseMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("nats_kv_lease")

	return &leaseMetrics{
		operations: scoped.MustCounter(metrics.MetricOpts{
			Name:       "operations_total",
			Help:       "Total number of lease operations.",
			LabelNames: []string{"op", "result"},
		}),
		leaseHeld: scoped.MustGauge(metrics.MetricOpts{
			Name: "lease_held",
			Help: "Whether the lease is currently held (1=held, 0=not held).",
		}),
	}
}
