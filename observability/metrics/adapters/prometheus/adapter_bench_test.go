// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prometheus_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"

	prometheusadapter "github.com/altessa-s/go-atlas/observability/metrics/adapters/prometheus"
)

func newBenchAdapter(b *testing.B) *prometheusadapter.Adapter {
	b.Helper()
	a := prometheusadapter.New(prometheusadapter.WithRegisterer(prometheus.NewRegistry()))
	for _, d := range []adapters.Desc{
		{Name: "requests_total", Help: "bench", Type: adapters.TypeCounter, LabelNames: []string{"method"}},
		{Name: "latency_seconds", Help: "bench", Type: adapters.TypeHistogram, LabelNames: []string{"method"}},
	} {
		if err := a.Register(&d); err != nil {
			b.Fatal(err)
		}
	}
	return a
}

func BenchmarkRecordCounter(b *testing.B) {
	a := newBenchAdapter(b)
	labels := map[string]string{"method": "GET"}

	b.ReportAllocs()
	for b.Loop() {
		a.RecordCounter("requests_total", labels, 1)
	}
}

func BenchmarkRecordHistogram(b *testing.B) {
	a := newBenchAdapter(b)
	labels := map[string]string{"method": "GET"}

	b.ReportAllocs()
	for b.Loop() {
		a.RecordHistogram("latency_seconds", labels, 0.42)
	}
}

func BenchmarkBoundCounterAdd(b *testing.B) {
	a := newBenchAdapter(b)
	bound, ok := a.BindCounter("requests_total", map[string]string{"method": "GET"})
	if !ok {
		b.Fatal("bind failed")
	}

	b.ReportAllocs()
	for b.Loop() {
		bound.Add(1)
	}
}

func BenchmarkBoundHistogramObserve(b *testing.B) {
	a := newBenchAdapter(b)
	bound, ok := a.BindHistogram("latency_seconds", map[string]string{"method": "GET"})
	if !ok {
		b.Fatal("bind failed")
	}

	b.ReportAllocs()
	for b.Loop() {
		bound.Observe(0.42)
	}
}
