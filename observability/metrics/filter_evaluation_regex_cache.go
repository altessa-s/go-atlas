// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

// RegexCacheStatsSnapshot holds a point-in-time snapshot of regex pattern cache
// performance counters, mirroring [filter.RegexCacheStats] without importing
// the filter package (which would create an import cycle).
type RegexCacheStatsSnapshot struct {
	Hits   uint64
	Misses uint64
	Size   int64
}

// HitRate returns the cache hit rate as a float64 in [0.0, 1.0].
// Returns 0 when there have been no lookups.
func (s RegexCacheStatsSnapshot) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total)
}

// RegexCacheMetrics exposes the CEL filter regex pattern cache performance
// counters as Prometheus-compatible metrics. The regex cache compiles and
// stores regular expressions used in filter `matches()` calls.
//
// Because data/filter imports observability/metrics, the bridge uses
// callback functions to avoid an import cycle:
//
//	rcm := metrics.NewRegexCacheMetrics(collector,
//	    filter.RegexCacheStatsSnapshot, // stats provider
//	    filter.ResetRegexCacheStats,    // stats resetter
//	)
//	sched.Register(ctx, scheduler.TaskConfig{
//	    ID:       "regex-cache-metrics",
//	    Schedule: "0 */1 * * * *",
//	    Func:     func(ctx context.Context) error { rcm.Report(); return nil },
//	})
type RegexCacheMetrics struct {
	statsFunc func() RegexCacheStatsSnapshot
	resetFunc func()

	hitsTotal   Counter
	missesTotal Counter
	currentSize Gauge
	hitRate     Gauge
}

// NewRegexCacheMetrics creates a [RegexCacheMetrics] that reports stats from
// the regex pattern cache. The statsFunc and resetFunc callbacks decouple this
// bridge from the filter package, avoiding import cycles.
//
// If c is nil, [Noop] is used and all operations become zero-cost no-ops.
func NewRegexCacheMetrics(c Collector, statsFunc func() RegexCacheStatsSnapshot, resetFunc func()) *RegexCacheMetrics {
	if c == nil {
		c = Noop()
	}

	scoped := c.WithSubsystem("regex_cache")

	return &RegexCacheMetrics{
		statsFunc: statsFunc,
		resetFunc: resetFunc,
		hitsTotal: scoped.MustCounter(MetricOpts{
			Name: "hits_total",
			Help: "Total number of regex pattern cache hits.",
		}),
		missesTotal: scoped.MustCounter(MetricOpts{
			Name: "misses_total",
			Help: "Total number of regex pattern cache misses requiring compilation.",
		}),
		currentSize: scoped.MustGauge(MetricOpts{
			Name: "current_size",
			Help: "Current number of compiled regex patterns in the cache.",
		}),
		hitRate: scoped.MustGauge(MetricOpts{
			Name: "hit_rate",
			Help: "Regex pattern cache hit rate (hits / total lookups).",
		}),
	}
}

// Report reads the current regex cache stats snapshot and pushes delta
// counters and absolute gauges into the metrics collector.
func (m *RegexCacheMetrics) Report() {
	stats := m.statsFunc()

	m.hitsTotal.Add(float64(stats.Hits))
	m.missesTotal.Add(float64(stats.Misses))
	m.currentSize.Set(float64(stats.Size))
	m.hitRate.Set(stats.HitRate())

	m.resetFunc()
}
