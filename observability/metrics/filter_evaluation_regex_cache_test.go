// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

func TestNewRegexCacheMetrics_NilCollector(t *testing.T) {
	t.Parallel()

	rcm := metrics.NewRegexCacheMetrics(nil,
		func() metrics.RegexCacheStatsSnapshot { return metrics.RegexCacheStatsSnapshot{} },
	)
	require.NotNil(t, rcm)

	// Should not panic with noop collector.
	rcm.Report()
}

func TestRegexCacheStatsSnapshot_HitRate(t *testing.T) {
	t.Parallel()

	s := metrics.RegexCacheStatsSnapshot{Hits: 8, Misses: 2}
	require.Equal(t, 0.8, s.HitRate())
}

func TestRegexCacheStatsSnapshot_HitRateZero(t *testing.T) {
	t.Parallel()

	s := metrics.RegexCacheStatsSnapshot{}
	require.Equal(t, float64(0), s.HitRate())
}

// OB-T12: concurrent RegexCacheMetrics.Report calls under -race must not double-count,
// and an upstream reset (100 -> 30) must add cur (yielding 130).
func TestRegexCacheMetrics_ConcurrentAndReset(t *testing.T) {
	t.Parallel()

	t.Run("concurrent_report_totals_one_snapshot", func(t *testing.T) {
		t.Parallel()
		tc := metricstest.NewTestCollector()
		rcm := metrics.NewRegexCacheMetrics(tc, func() metrics.RegexCacheStatsSnapshot {
			return metrics.RegexCacheStatsSnapshot{Hits: 50, Misses: 10, Size: 5}
		})

		start := make(chan struct{})
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				<-start
				rcm.Report()
			})
		}
		close(start)
		wg.Wait()

		require.InDelta(t, 50.0, metricstest.GetCounterValue(t, tc, "test_regex_cache_hits_total"), 0)
		require.InDelta(t, 10.0, metricstest.GetCounterValue(t, tc, "test_regex_cache_misses_total"), 0)
	})

	t.Run("reset_100_to_30_yields_130", func(t *testing.T) {
		t.Parallel()
		tc := metricstest.NewTestCollector()
		var step atomic.Int32
		rcm := metrics.NewRegexCacheMetrics(tc, func() metrics.RegexCacheStatsSnapshot {
			if step.Add(1) == 1 {
				return metrics.RegexCacheStatsSnapshot{Hits: 100, Misses: 20, Size: 10}
			}
			return metrics.RegexCacheStatsSnapshot{Hits: 30, Misses: 5, Size: 3}
		})

		rcm.Report()
		rcm.Report()

		require.InDelta(t, 130.0, metricstest.GetCounterValue(t, tc, "test_regex_cache_hits_total"), 0)
		require.InDelta(t, 25.0, metricstest.GetCounterValue(t, tc, "test_regex_cache_misses_total"), 0)
	})
}
