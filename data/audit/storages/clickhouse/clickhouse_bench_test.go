// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
)

func BenchmarkToRowInto(b *testing.B) {
	event := fullEvent()
	var row eventRow

	b.ReportAllocs()

	for b.Loop() {
		if err := toRowInto(&row, event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFromRow(b *testing.B) {
	row, err := toRow(fullEvent())
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = fromRow(row)
	}
}

func BenchmarkColumnBufferAppendRow(b *testing.B) {
	const batch = 1000
	row, err := toRow(fullEvent())
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		buf := newColumnBuffer(batch)
		for range batch {
			buf.appendRow(row)
		}
	}
}

func BenchmarkBuildWhere(b *testing.B) {
	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	query := &audit.Query{
		StartTime:    &start,
		EndTime:      &end,
		ActorID:      "user-42",
		ResourceType: "invoice",
		EventType:    audit.EventTypeDataChange,
		Status:       audit.ResultStatusSuccess,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = buildWhere(query, nil)
	}
}

func BenchmarkSelectStatement(b *testing.B) {
	start := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	query := &audit.Query{StartTime: &start, ActorID: "user-42", Limit: 100}
	s := &Storage{opts: newOptions()}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = s.selectStatement(query, nil)
	}
}
