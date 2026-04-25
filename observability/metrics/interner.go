// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// InternerMetrics exposes string [corestrings.Interner] performance counters
// as Prometheus-compatible metrics. Counters track hot/cold cache hits, misses,
// and evictions; gauges report current size and hit rates.
//
// Create with [NewInternerMetrics] and call [InternerMetrics.Report] on a
// periodic schedule (e.g., via service/scheduler) to push the latest snapshot
// into the configured [Collector]:
//
//	im := metrics.NewInternerMetrics(collector, strings.GlobalInterner())
//	sched.Register(ctx, scheduler.TaskConfig{
//	    ID:       "interner-metrics",
//	    Schedule: "0 */1 * * * *",
//	    Func:     im.Report,
//	})
type InternerMetrics struct {
	interner *corestrings.Interner

	hotHitsTotal   Counter
	coldHitsTotal  Counter
	missesTotal    Counter
	evictionsTotal Counter
	currentSize    Gauge
	hitRate        Gauge
	hotHitRate     Gauge
}

// NewInternerMetrics creates an [InternerMetrics] that reports stats from the
// given [corestrings.Interner] through the provided [Collector]. If c is nil,
// [Noop] is used and all operations become zero-cost no-ops.
func NewInternerMetrics(c Collector, interner *corestrings.Interner) *InternerMetrics {
	if c == nil {
		c = Noop()
	}

	scoped := c.WithSubsystem("interner")

	return &InternerMetrics{
		interner: interner,
		hotHitsTotal: scoped.MustCounter(MetricOpts{
			Name: "hot_hits_total",
			Help: "Total number of lookups served from the hot cache (atomic slots).",
		}),
		coldHitsTotal: scoped.MustCounter(MetricOpts{
			Name: "cold_hits_total",
			Help: "Total number of lookups served from the cold cache (sync.Map).",
		}),
		missesTotal: scoped.MustCounter(MetricOpts{
			Name: "misses_total",
			Help: "Total number of lookups that required creating a new entry.",
		}),
		evictionsTotal: scoped.MustCounter(MetricOpts{
			Name: "evictions_total",
			Help: "Total number of entries removed by background LRU eviction.",
		}),
		currentSize: scoped.MustGauge(MetricOpts{
			Name: "current_size",
			Help: "Current number of interned strings in the cache.",
		}),
		hitRate: scoped.MustGauge(MetricOpts{
			Name: "hit_rate",
			Help: "Overall cache hit rate (hot + cold hits / total lookups).",
		}),
		hotHitRate: scoped.MustGauge(MetricOpts{
			Name: "hot_hit_rate",
			Help: "Hot cache hit rate (hot hits / total lookups).",
		}),
	}
}

// Report reads the current [corestrings.InternerStats] snapshot and pushes
// delta counters and absolute gauges into the metrics collector. It is
// designed to be called periodically (e.g., every minute via scheduler).
//
// Counter metrics are reported as deltas since the last Report call; gauge
// metrics reflect the instantaneous value at the time of the call.
func (im *InternerMetrics) Report() {
	stats := im.interner.Stats()

	// Counters: report cumulative values — the metrics backend handles delta.
	im.hotHitsTotal.Add(float64(stats.HotHits))
	im.coldHitsTotal.Add(float64(stats.ColdHits))
	im.missesTotal.Add(float64(stats.Misses))
	im.evictionsTotal.Add(float64(stats.Evictions))

	// Gauges: absolute point-in-time values.
	im.currentSize.Set(float64(stats.CurrentSize))
	im.hitRate.Set(stats.HitRate())
	im.hotHitRate.Set(stats.HotHitRate())

	// Reset stats so next Report() captures only the delta window.
	im.interner.ResetStats()
}
