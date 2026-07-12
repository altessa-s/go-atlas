// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/observability/metrics/adapters"
	"github.com/altessa-s/go-atlas/observability/metrics/adapters/memory"
)

func BenchmarkRecordCounter(b *testing.B) {
	a := memory.New()
	if err := a.Register(&adapters.Desc{
		Name: "requests_total", Type: adapters.TypeCounter, LabelNames: []string{"method"},
	}); err != nil {
		b.Fatal(err)
	}
	labels := map[string]string{"method": "GET"}

	b.ReportAllocs()
	for b.Loop() {
		a.RecordCounter("requests_total", labels, 1)
	}
}

func BenchmarkRecordHistogram(b *testing.B) {
	a := memory.New()
	if err := a.Register(&adapters.Desc{
		Name: "request_duration_seconds", Type: adapters.TypeHistogram, LabelNames: []string{"method"},
	}); err != nil {
		b.Fatal(err)
	}
	labels := map[string]string{"method": "GET"}

	b.ReportAllocs()
	for b.Loop() {
		a.RecordHistogram("request_duration_seconds", labels, 0.25)
	}
}
