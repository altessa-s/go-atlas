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
//	    filter.RegexCacheStatsSnapshot, // stats provider (cumulative)
//	    nil,                            // resetFunc (legacy, optional)
//	)
//	sched.Register(ctx, scheduler.TaskConfig{
//	    ID:       "regex-cache-metrics",
//	    Schedule: "0 */1 * * * *",
//	    Func:     func(ctx context.Context) error { rcm.Report(); return nil },
//	})
//
// statsFunc must return CUMULATIVE counters — [RegexCacheMetrics.Report]
// tracks previous values internally and computes deltas, so the upstream
// is no longer required to reset between reports.
type RegexCacheMetrics struct {
	statsFunc func() RegexCacheStatsSnapshot
	resetFunc func()

	// Previous-snapshot bookkeeping for delta computation. Holds the
	// last cumulative values observed by Report; the next Report adds
	// only the difference. Removes the previous reliance on resetFunc
	// being called between reports — the old contract conflated "stats
	// provider" with "consumer reset hook" and any caller that forgot
	// to wire resetFunc would have inflated the counters monotonically.
	prevHits   uint64
	prevMisses uint64

	hitsTotal   Counter
	missesTotal Counter
	currentSize Gauge
	hitRate     Gauge
}

// NewRegexCacheMetrics creates a [RegexCacheMetrics] that reports stats from
// the regex pattern cache. statsFunc must return cumulative counters; the
// returned metrics tracks deltas internally. resetFunc is kept for
// backwards compatibility — pass nil for new code; if non-nil it is
// invoked after each Report to give legacy callers a hook for their own
// bookkeeping, but the metric output no longer depends on it.
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

// Report reads the current regex cache stats snapshot and pushes the
// delta since the previous Report into the counters. Gauges are absolute.
// A backwards step (cumulative counter went down — typical when the
// upstream cache is explicitly cleared) is treated as a counter reset:
// the new value becomes the baseline and no negative delta is published.
func (m *RegexCacheMetrics) Report() {
	stats := m.statsFunc()

	hitsDelta, prevHits := deltaCounter(m.prevHits, stats.Hits)
	missesDelta, prevMisses := deltaCounter(m.prevMisses, stats.Misses)

	m.prevHits = prevHits
	m.prevMisses = prevMisses

	if hitsDelta > 0 {
		m.hitsTotal.Add(float64(hitsDelta))
	}
	if missesDelta > 0 {
		m.missesTotal.Add(float64(missesDelta))
	}
	m.currentSize.Set(float64(stats.Size))
	m.hitRate.Set(stats.HitRate())

	if m.resetFunc != nil {
		m.resetFunc()
	}
}

// deltaCounter computes the additive delta between two snapshots of a
// monotonic counter. A backwards step (cur < prev) is treated as a
// reset — caller should re-baseline at cur rather than publishing a
// negative delta. Returns (delta, newPrev).
func deltaCounter(prev, cur uint64) (delta uint64, newPrev uint64) {
	if cur < prev {
		return 0, cur
	}
	return cur - prev, cur
}
