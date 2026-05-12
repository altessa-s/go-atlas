// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
)

// TestCollector pairs a [metrics.Collector] with an in-memory
// [memory.Adapter] so tests can register metrics through the standard
// collector API and read back accumulated values without standing up a
// Prometheus registry.
//
// Methods of the embedded Collector are forwarded directly, so a
// *TestCollector can be passed anywhere a [metrics.Collector] is
// expected. Lookup methods of [memory.Adapter] are likewise forwarded
// for assertions in tests.
type TestCollector struct {
	metrics.Collector
	*memory.Adapter
}

// NewTestCollector creates a fresh [TestCollector] with service name
// "test". Each call returns an independent collector so tests can run
// in parallel without sharing metric state.
func NewTestCollector() *TestCollector {
	mem := memory.New()
	coll := metrics.New(metrics.WithServiceName("test"), metrics.WithAdapter(mem))
	return &TestCollector{Collector: coll, Adapter: mem}
}

// GatherMetric reports whether a metric with the given name has been
// registered on tc. Use it to assert presence or absence of a metric
// without caring about its accumulated value.
func GatherMetric(t *testing.T, tc *TestCollector, name string) bool {
	t.Helper()
	return tc.Exists(name)
}

// GetCounterValue returns the accumulated value of the named counter
// for the given label pairs. labelPairs must be a flat list of
// alternating key/value strings ("method", "GET", "status_class", "2xx"),
// matching the labels supplied when the counter was observed. Returns
// 0 when the metric or label combination is unknown.
func GetCounterValue(t *testing.T, tc *TestCollector, name string, labelPairs ...string) float64 {
	t.Helper()
	return tc.CounterValue(name, labelsFromPairs(labelPairs))
}

// GetGaugeValue returns the most recently set value of the named gauge
// for the given label pairs. See [GetCounterValue] for the labelPairs
// format. Returns 0 when the metric or label combination is unknown.
func GetGaugeValue(t *testing.T, tc *TestCollector, name string, labelPairs ...string) float64 {
	t.Helper()
	return tc.GaugeValue(name, labelsFromPairs(labelPairs))
}

// GetHistogramCount returns the number of observations recorded for
// the named histogram and label combination. See [GetCounterValue] for
// the labelPairs format. Returns 0 when the metric or label combination
// is unknown.
func GetHistogramCount(t *testing.T, tc *TestCollector, name string, labelPairs ...string) uint64 {
	t.Helper()
	return tc.HistogramCount(name, labelsFromPairs(labelPairs))
}

// labelsFromPairs converts a flat key/value slice into a map. An odd
// number of elements drops the trailing key, matching the legacy
// permissive behavior callers relied on.
func labelsFromPairs(pairs []string) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i]] = pairs[i+1]
	}
	return out
}
