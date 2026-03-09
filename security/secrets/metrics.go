// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// secretsMetrics holds all Prometheus metrics for the secret manager.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type secretsMetrics struct {
	cacheHits          metrics.Counter
	cacheMisses        metrics.Counter
	fetchDuration      metrics.Timer
	updateCycleDuration metrics.Timer
	updateCycleErrors  metrics.Counter
	cacheSize          metrics.Gauge
}

func newSecretsMetrics(c metrics.Collector) *secretsMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("secrets")

	return &secretsMetrics{
		cacheHits: scoped.MustCounter(metrics.MetricOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits.",
		}),
		cacheMisses: scoped.MustCounter(metrics.MetricOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses.",
		}),
		fetchDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "fetch_duration_seconds",
				Help: "Duration of secret fetch operations in seconds.",
			},
		}),
		updateCycleDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "update_cycle_duration_seconds",
				Help: "Duration of cache update cycles in seconds.",
			},
		}),
		updateCycleErrors: scoped.MustCounter(metrics.MetricOpts{
			Name: "update_cycle_errors_total",
			Help: "Total number of failed update cycles.",
		}),
		cacheSize: scoped.MustGauge(metrics.MetricOpts{
			Name: "cache_size",
			Help: "Current number of entries in the cache.",
		}),
	}
}
