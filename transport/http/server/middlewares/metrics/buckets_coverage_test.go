// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/metrics"
)

// Custom histogram buckets must be strictly increasing: a misconfigured
// bucket list fails loudly at construction instead of producing a broken
// histogram at runtime.
func TestBucketOptions_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		opt       func([]float64) metrics.Option
		buckets   []float64
		wantPanic bool
	}{
		{name: "duration increasing", opt: metrics.WithDurationBuckets, buckets: []float64{0.01, 0.1, 1}},
		{name: "duration empty keeps defaults", opt: metrics.WithDurationBuckets, buckets: nil},
		{name: "duration decreasing", opt: metrics.WithDurationBuckets, buckets: []float64{1, 0.1}, wantPanic: true},
		{name: "duration duplicate", opt: metrics.WithDurationBuckets, buckets: []float64{0.1, 0.1}, wantPanic: true},
		{name: "size increasing", opt: metrics.WithSizeBuckets, buckets: []float64{1024, 4096}},
		{name: "size decreasing", opt: metrics.WithSizeBuckets, buckets: []float64{4096, 1024}, wantPanic: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			build := func() {
				metrics.New(metrics.WithMeter(metricstest.NewTestCollector()), metrics.WithEnableSizeMetrics(), tc.opt(tc.buckets))
			}
			if tc.wantPanic {
				require.Panics(t, build)
				return
			}
			require.NotPanics(t, build)
		})
	}
}
