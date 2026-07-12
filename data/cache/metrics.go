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
//
// Labeled metrics are pre-bound to the cache_name label at construction so
// per-operation recording does not rebuild the label set.
type cacheMetrics struct {
	hits             metrics.Counter
	misses           metrics.Counter
	errors           metrics.Counter
	negativeHits     metrics.Counter
	evictions        metrics.Counter
	size             metrics.Gauge
	writeDuration    metrics.Timer
	fallbackDuration metrics.Timer
}

func newCacheMetrics(c metrics.Collector, cacheName string) *cacheMetrics {
	if c == nil {
		c = metrics.Noop()
	}

	scoped := c.WithSubsystem("cache")
	labels := metrics.Labels{"cache_name": cacheName}

	return &cacheMetrics{
		hits: scoped.MustCounter(metrics.MetricOpts{
			Name:       "hits_total",
			Help:       "Total number of cache hits.",
			LabelNames: []string{"cache_name"},
		}).WithLabels(labels),
		misses: scoped.MustCounter(metrics.MetricOpts{
			Name:       "misses_total",
			Help:       "Total number of cache misses.",
			LabelNames: []string{"cache_name"},
		}).WithLabels(labels),
		errors: scoped.MustCounter(metrics.MetricOpts{
			Name:       "errors_total",
			Help:       "Total number of cache operation errors.",
			LabelNames: []string{"cache_name"},
		}).WithLabels(labels),
		negativeHits: scoped.MustCounter(metrics.MetricOpts{
			Name:       "negative_hits_total",
			Help:       "Total number of negative cache hits.",
			LabelNames: []string{"cache_name"},
		}).WithLabels(labels),
		evictions: scoped.MustCounter(metrics.MetricOpts{
			Name:       "evictions_total",
			Help:       "Total number of cache evictions.",
			LabelNames: []string{"cache_name"},
		}).WithLabels(labels),
		size: scoped.MustGauge(metrics.MetricOpts{
			Name:       "size",
			Help:       "Current number of entries in the cache.",
			LabelNames: []string{"cache_name"},
		}).WithLabels(labels),
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
