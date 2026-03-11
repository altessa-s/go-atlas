// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// httpClientMetrics holds all Prometheus metrics for the HTTP client.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type httpClientMetrics struct {
	requestsTotal    metrics.Counter
	requestErrors    metrics.Counter
	retries          metrics.Counter
	requestDuration  metrics.Timer
	circuitBreakerTrips metrics.Counter
}

func newHTTPClientMetrics(c metrics.Collector) *httpClientMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("http_client")

	return &httpClientMetrics{
		requestsTotal: scoped.MustCounter(metrics.MetricOpts{
			Name: "requests_total",
			Help: "Total number of HTTP requests completed.",
		}),
		requestErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "request_errors_total",
			Help: "Total number of HTTP request errors after all retries.",
		}),
		retries: scoped.MustCounter(metrics.MetricOpts{
			Name: "retries_total",
			Help: "Total number of HTTP request retry attempts.",
		}),
		requestDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "request_duration_seconds",
				Help: "Duration of HTTP requests including retries in seconds.",
			},
		}),
		circuitBreakerTrips: scoped.MustCounter(metrics.MetricOpts{
			Name: "circuit_breaker_trips_total",
			Help: "Total number of circuit breaker trip events.",
		}),
	}
}
