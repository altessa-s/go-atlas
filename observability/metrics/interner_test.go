// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func TestNewInternerMetrics_NilCollector(t *testing.T) {
	interner := corestrings.NewInterner(100)
	im := metrics.NewInternerMetrics(nil, interner)
	require.NotNil(t, im)

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
	require.Zero(t, stats.HotHits, "stats not reset after Report")
	require.Zero(t, stats.ColdHits, "stats not reset after Report")
	require.Zero(t, stats.Misses, "stats not reset after Report")
}
