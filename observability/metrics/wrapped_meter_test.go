// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
)

func TestCheckedRegistration_ThroughWrappedMeter(t *testing.T) {
	t.Parallel()

	c := metricstest.NewTestCollector()
	opts := metrics.MetricOpts{Name: "conflict"}
	c.Gauge(opts)

	counter, counterErr := metrics.RegisterCounter(c, opts)
	gauge, gaugeErr := metrics.RegisterGauge(c, metrics.MetricOpts{Name: opts.Name, Help: "changed"})
	histogram, histogramErr := metrics.RegisterHistogram(c, metrics.HistogramOpts{MetricOpts: opts})
	timer, timerErr := metrics.RegisterTimer(c, metrics.HistogramOpts{MetricOpts: opts})
	for _, err := range []error{counterErr, gaugeErr, histogramErr, timerErr} {
		require.ErrorIs(t, err, metrics.ErrDescriptorConflict)
	}
	require.True(t, counter == nil && gauge == nil && histogram == nil && timer == nil)
	require.Panics(t, func() { metrics.MustCounter(c, opts) })
}
