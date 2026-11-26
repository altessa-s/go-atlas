// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

func TestNewInternerMetrics_NilCollector(t *testing.T) {
	t.Parallel()

	interner := intern.New(100)
	im := metrics.NewInternerMetrics(nil, interner)
	require.NotNil(t, im)

	// Should not panic with noop collector.
	im.Report()
}

func TestInternerMetrics_Report(t *testing.T) {
	t.Parallel()

	interner := intern.New(100)
	tc := metricstest.NewTestCollector()
	im := metrics.NewInternerMetrics(tc, interner)

	// Generate some cache activity.
	interner.String("alpha") // miss
	interner.String("alpha") // hit
	interner.String("beta")  // miss

	im.Report()

	// Cumulative interner stats must remain intact for other readers.
	stats := interner.Stats()
	require.Equal(t, uint64(3), stats.TotalLookups())

	// Repeating Report with no new lookups adds zero delta.
	im.Report()
	total := metricstest.GetCounterValue(t, tc, "test_interner_hot_hits_total") +
		metricstest.GetCounterValue(t, tc, "test_interner_cold_hits_total") +
		metricstest.GetCounterValue(t, tc, "test_interner_misses_total")
	require.InDelta(t, 3.0, total, 0)
}

// OB-T14: InternerMetrics.Report must never reset the shared interner and must sum to total lookups.
func TestInternerMetrics_CumulativeDeltaNeverResets(t *testing.T) {
	t.Parallel()

	interner := intern.New(1024)
	tc := metricstest.NewTestCollector()
	im := metrics.NewInternerMetrics(tc, interner)

	const workers = 8
	const perWorker = 200
	done := make(chan struct{})

	var reportWG sync.WaitGroup
	reportWG.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
				im.Report()
			}
		}
	})

	var lookupWG sync.WaitGroup
	for w := range workers {
		lookupWG.Go(func() {
			for i := range perWorker {
				interner.String(fmt.Sprintf("k-%d-%d", w%2, i%25))
			}
		})
	}
	lookupWG.Wait()
	close(done)
	reportWG.Wait()

	im.Report()

	const totalLookups = workers * perWorker
	stats := interner.Stats()
	require.Equal(t, uint64(totalLookups), stats.TotalLookups(), "Report must not call ResetStats on the interner")

	reported := metricstest.GetCounterValue(t, tc, "test_interner_hot_hits_total") +
		metricstest.GetCounterValue(t, tc, "test_interner_cold_hits_total") +
		metricstest.GetCounterValue(t, tc, "test_interner_misses_total")
	require.InDelta(t, float64(totalLookups), reported, 0)
}
