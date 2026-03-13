// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/metrics"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestNewInternerMetrics_NilCollector(t *testing.T) {
	interner := corestrings.NewInterner(100)
	im := metrics.NewInternerMetrics(nil, interner)
	if im == nil {
		t.Fatal("NewInternerMetrics(nil, ...) returned nil")
	}

	// Should not panic with noop collector.
	im.Report()
}

func TestInternerMetrics_Report(t *testing.T) {
	interner := corestrings.NewInterner(100)
	im := metrics.NewInternerMetrics(metrics.Noop(), interner)

	// Generate some cache activity.
	interner.String("alpha")
	interner.String("alpha") // cold hit
	interner.String("beta")  // miss

	im.Report()

	// After Report, stats should be reset.
	stats := interner.Stats()
	if stats.HotHits != 0 || stats.ColdHits != 0 || stats.Misses != 0 {
		t.Errorf("stats not reset after Report: %+v", stats)
	}
}
