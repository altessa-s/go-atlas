// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/metrics"
)

func TestNewRegexCacheMetrics_NilCollector(t *testing.T) {
	rcm := metrics.NewRegexCacheMetrics(nil,
		func() metrics.RegexCacheStatsSnapshot { return metrics.RegexCacheStatsSnapshot{} },
		func() {},
	)
	if rcm == nil {
		t.Fatal("NewRegexCacheMetrics(nil, ...) returned nil")
	}

	// Should not panic with noop collector.
	rcm.Report()
}

func TestRegexCacheMetrics_Report(t *testing.T) {
	var resetCalled bool
	statsFunc := func() metrics.RegexCacheStatsSnapshot {
		return metrics.RegexCacheStatsSnapshot{Hits: 10, Misses: 2, Size: 5}
	}
	resetFunc := func() { resetCalled = true }

	rcm := metrics.NewRegexCacheMetrics(metrics.Noop(), statsFunc, resetFunc)
	rcm.Report()

	if !resetCalled {
		t.Error("Report() did not call resetFunc")
	}
}

func TestRegexCacheStatsSnapshot_HitRate(t *testing.T) {
	s := metrics.RegexCacheStatsSnapshot{Hits: 8, Misses: 2}
	if got := s.HitRate(); got != 0.8 {
		t.Errorf("HitRate() = %f, want 0.8", got)
	}
}

func TestRegexCacheStatsSnapshot_HitRateZero(t *testing.T) {
	s := metrics.RegexCacheStatsSnapshot{}
	if got := s.HitRate(); got != 0 {
		t.Errorf("HitRate() = %f, want 0", got)
	}
}
