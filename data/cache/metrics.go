// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

import (
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// cacheMetrics holds all Prometheus metrics for the Cache.
// When no [metrics.Collector] is provided, [metrics.Noop] is used and
// all methods become zero-cost no-ops.
type cacheMetrics struct {
	hits             metrics.Counter
	misses           metrics.Counter
	errors           metrics.Counter
	writeDuration    metrics.Timer
	fallbackDuration metrics.Timer
}

func newCacheMetrics(c metrics.Collector) *cacheMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("cache")

	return &cacheMetrics{
		hits: scoped.MustCounter(metrics.MetricOpts{
			Name: "hits_total",
			Help: "Total number of cache hits.",
		}),
		misses: scoped.MustCounter(metrics.MetricOpts{
			Name: "misses_total",
			Help: "Total number of cache misses.",
		}),
		errors: scoped.MustCounter(metrics.MetricOpts{
			Name: "errors_total",
			Help: "Total number of cache operation errors.",
		}),
		writeDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "write_duration_seconds",
				Help: "Duration of cache write operations in seconds.",
			},
		}),
		fallbackDuration: scoped.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name: "fallback_duration_seconds",
				Help: "Duration of fallback function execution in seconds.",
			},
		}),
	}
}
