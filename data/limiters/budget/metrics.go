// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package budget

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// limiterMetrics holds all Prometheus metrics for the budget [Limiter].
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type limiterMetrics struct {
	requestsAllowed  metrics.Counter
	requestsRejected metrics.Counter
	limitCheckErrors metrics.Counter
}

func newLimiterMetrics(c metrics.Collector) *limiterMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("budget_limiter")

	return &limiterMetrics{
		requestsAllowed: scoped.MustCounter(metrics.MetricOpts{
			Name: "requests_allowed_total",
			Help: "Total number of requests allowed by the budget limiter.",
		}),
		requestsRejected: scoped.MustCounter(metrics.MetricOpts{
			Name: "requests_rejected_total",
			Help: "Total number of requests rejected due to budget exhaustion.",
		}),
		limitCheckErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "limit_check_errors_total",
			Help: "Total number of errors during budget limit checks.",
		}),
	}
}
