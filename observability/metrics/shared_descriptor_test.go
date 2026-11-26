// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"

	prometheusadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

func TestCollector_DescriptorConflictAcrossCollectors(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	prom := prometheusadapter.New(prometheusadapter.WithRegistry(reg))
	for _, tc := range []struct {
		name          string
		first, second adapters.Adapter
	}{
		{name: "memory", first: memory.New()},
		{name: "multi", first: adapters.NewMultiAdapter(memory.New())},
		{name: "prometheus", first: prom},
		{name: "shared_registry", first: prom, second: prometheusadapter.New(prometheusadapter.WithRegistry(reg))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			second := tc.second
			if second == nil {
				second = tc.first
			}
			c1 := metrics.New(metrics.WithAdapter(tc.first))
			c2 := metrics.New(metrics.WithAdapter(second))
			opts := metrics.HistogramOpts{Name: "latency_" + tc.name, LabelNames: []string{"route"}, Buckets: []float64{1, 2}}
			_, err := metrics.RegisterHistogram(c1, opts)
			require.NoError(t, err)
			_, err = metrics.RegisterTimer(c2, opts)
			require.ErrorIs(t, err, metrics.ErrDescriptorConflict)
		})
	}
}

func TestCollector_BucketConflictAcrossPrometheusAdapters(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	c1 := metrics.New(metrics.WithAdapter(prometheusadapter.New(prometheusadapter.WithRegistry(reg))))
	c2 := metrics.New(metrics.WithAdapter(prometheusadapter.New(prometheusadapter.WithRegistry(reg))))
	opts := metrics.HistogramOpts{Name: "latency", LabelNames: []string{"route"}, Buckets: []float64{1, 2}}
	_, err := metrics.RegisterHistogram(c1, opts)
	require.NoError(t, err)
	different := opts
	different.Buckets = []float64{2, 3}
	_, err = metrics.RegisterHistogram(c2, different)
	require.ErrorIs(t, err, metrics.ErrDescriptorConflict)

	h := metrics.MustHistogram(c2, opts)
	h.WithLabels(metrics.Labels{"route": "GET"}).Observe(1)
	families, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Equal(t, uint64(1), families[0].GetMetric()[0].GetHistogram().GetBucket()[0].GetCumulativeCount())
}
