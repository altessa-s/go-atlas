// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// vaultMetrics holds all Prometheus metrics for the Vault client.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type vaultMetrics struct {
	renewalAttempts metrics.Counter
	renewalErrors   metrics.Counter
	renewalDuration metrics.Timer
}

func newVaultMetrics(c metrics.Collector) *vaultMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("vault")

	return &vaultMetrics{
		renewalAttempts: scoped.MustCounter(metrics.MetricOpts{
			Name: "renewal_attempts_total",
			Help: "Total number of Vault token renewal attempts.",
		}),
		renewalErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "renewal_errors_total",
			Help: "Total number of Vault token renewal failures.",
		}),
		renewalDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "renewal_duration_seconds",
				Help: "Duration of Vault token renewal operations in seconds.",
			},
		}),
	}
}
