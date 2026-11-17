// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/audit"
)

// BenchmarkToModel converts one event for StoreBatch.
func BenchmarkToModel(b *testing.B) {
	e := fullEvent()
	for b.Loop() {
		toModel(e)
	}
}

// BenchmarkBuildFilter builds the filter of a query by time range and trace.
func BenchmarkBuildFilter(b *testing.B) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	q := &audit.Query{StartTime: &start, EndTime: &end, TraceID: "trace-1"}
	for b.Loop() {
		buildFilter(q)
	}
}
