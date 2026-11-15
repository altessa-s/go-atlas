// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package aipmeter_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/filter/parser"

	"github.com/altessa-s/go-atlas/observability/metrics/aipmeter"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	aipmetrics "github.com/altessa-s/go-aip/metrics"
)

func TestNew_NilIsNoop(t *testing.T) {
	t.Parallel()

	require.Equal(t, aipmetrics.Noop(), aipmeter.New(nil))

	var typedNil *metricstest.TestCollector
	require.Equal(t, aipmetrics.Noop(), aipmeter.New(typedNil))
}

func TestMeter_ForwardsInstruments(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	m := aipmeter.New(tc).WithSubsystem("filter")

	m.Counter(aipmetrics.MetricOpts{Name: "a_total"}).Inc()
	m.MustCounter(aipmetrics.MetricOpts{Name: "b_total"}).Add(2)
	stop := m.Timer(aipmetrics.HistogramOpts{MetricOpts: aipmetrics.MetricOpts{Name: "a_seconds"}}).Start()
	stop()
	m.MustTimer(aipmetrics.HistogramOpts{MetricOpts: aipmetrics.MetricOpts{Name: "b_seconds"}}).ObserveDuration(time.Millisecond)

	require.InDelta(t, 1.0, metricstest.GetCounterValue(t, tc, "test_filter_a_total"), 0)
	require.InDelta(t, 2.0, metricstest.GetCounterValue(t, tc, "test_filter_b_total"), 0)
	require.Equal(t, uint64(1), metricstest.GetHistogramCount(t, tc, "test_filter_a_seconds"))
	require.Equal(t, uint64(1), metricstest.GetHistogramCount(t, tc, "test_filter_b_seconds"))
}

// TestMeter_GoAIPParser pins the end-to-end path: a go-aip filter parser given
// the adapter records its parse errors into the go-atlas collector.
func TestMeter_GoAIPParser(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	p, err := parser.New(parser.WithCollector(aipmeter.New(tc)), parser.WithNoCache(true))
	require.NoError(t, err)

	_, err = p.Parse(t.Context(), `name ==`)
	require.Error(t, err)
	require.InDelta(t, 1.0, metricstest.GetCounterValue(t, tc, "test_filter_parse_errors_total"), 0)
}
