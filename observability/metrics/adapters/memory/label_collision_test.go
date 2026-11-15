// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
)

func TestLabelValuesCannotMergeSeries(t *testing.T) {
	t.Parallel()
	a := memory.New()
	first := map[string]string{"a": "x|b=y", "b": "z"}
	second := map[string]string{"a": "x", "b": "y|b=z"}
	for _, desc := range []adapters.Desc{
		{Name: "counter", Type: adapters.TypeCounter},
		{Name: "gauge", Type: adapters.TypeGauge},
		{Name: "histogram", Type: adapters.TypeHistogram},
	} {
		desc.LabelNames = []string{"a", "b"}
		require.NoError(t, a.Register(&desc))
	}
	a.RecordCounter("counter", first, 2)
	a.RecordCounter("counter", second, 3)
	a.RecordGauge("gauge", first, 2)
	a.RecordGauge("gauge", second, 3)
	a.RecordHistogram("histogram", first, 2)
	a.RecordHistogram("histogram", second, 3)
	require.Equal(t, float64(2), a.CounterValue("counter", first))
	require.Equal(t, float64(3), a.CounterValue("counter", second))
	require.Equal(t, float64(2), a.GaugeValue("gauge", first))
	require.Equal(t, float64(3), a.GaugeValue("gauge", second))
	require.Equal(t, uint64(1), a.HistogramCount("histogram", first))
	require.Equal(t, float64(2), a.HistogramSum("histogram", first))
	require.Equal(t, float64(3), a.HistogramSum("histogram", second))
}
