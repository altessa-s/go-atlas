// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics

import (
	"sync"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"
)

// InternerMetrics exposes string [intern.Interner] performance counters
// as Prometheus-compatible metrics. Counters track hot/cold cache hits, misses,
// and evictions; gauges report current size and hit rates.
//
// Create with [NewInternerMetrics] and call [InternerMetrics.Report] on a
// periodic schedule (e.g., via go-horai) to push the latest snapshot
// into the configured [Meter]:
//
//	im := metrics.NewInternerMetrics(collector, intern.Default())
//	sched.Register(ctx, schedtask.Config{
//	    ID:       "interner-metrics",
//	    Schedule: "0 */1 * * * *",
//	    Func:     func(ctx context.Context) error { im.Report(); return nil },
//	})
type InternerMetrics struct {
	interner *intern.Interner

	mu            sync.Mutex
	prevHotHits   uint64
	prevColdHits  uint64
	prevMisses    uint64
	prevEvictions uint64

	hotHitsTotal   Counter
	coldHitsTotal  Counter
	missesTotal    Counter
	evictionsTotal Counter
	currentSize    Gauge
	hitRate        Gauge
	hotHitRate     Gauge
}

// NewInternerMetrics creates an [InternerMetrics] that reports stats from the
// given [intern.Interner] through the provided [Meter]. If c is nil,
// [Noop] is used and all operations become zero-cost no-ops.
func NewInternerMetrics(c Meter, interner *intern.Interner) *InternerMetrics {
	if c == nil {
		c = Noop()
	}

	scoped := MustMeter{Meter: c.WithSubsystem("interner")}

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

// Report reads the current cumulative [intern.Stats] snapshot and pushes
// counter deltas since the previous Report and instantaneous gauges into the
// metrics collector without resetting the interner's stats. Safe for
// concurrent calls.
func (im *InternerMetrics) Report() {
	im.mu.Lock()
	defer im.mu.Unlock()

	stats := im.interner.Stats()

	hotDelta, prevHot := deltaCounter(im.prevHotHits, stats.HotHits)
	coldDelta, prevCold := deltaCounter(im.prevColdHits, stats.ColdHits)
	missesDelta, prevMisses := deltaCounter(im.prevMisses, stats.Misses)
	evictDelta, prevEvict := deltaCounter(im.prevEvictions, stats.Evictions)

	im.prevHotHits = prevHot
	im.prevColdHits = prevCold
	im.prevMisses = prevMisses
	im.prevEvictions = prevEvict

	if hotDelta > 0 {
		im.hotHitsTotal.Add(float64(hotDelta))
	}
	if coldDelta > 0 {
		im.coldHitsTotal.Add(float64(coldDelta))
	}
	if missesDelta > 0 {
		im.missesTotal.Add(float64(missesDelta))
	}
	if evictDelta > 0 {
		im.evictionsTotal.Add(float64(evictDelta))
	}

	im.currentSize.Set(float64(stats.CurrentSize))
	im.hitRate.Set(stats.HitRate())
	im.hotHitRate.Set(stats.HotHitRate())
}
