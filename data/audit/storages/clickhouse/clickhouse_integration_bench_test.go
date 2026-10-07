// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	auditclickhouse "github.com/altessa-s/go-atlas/data/audit/storages/clickhouse"
)

// benchEvents builds n events sharing a timestamp, which is what a real
// batch from the dispatcher looks like.
func benchEvents(n int) []*audit.Event {
	base := time.Now().UTC().Truncate(time.Millisecond)
	events := make([]*audit.Event, n)
	for i := range events {
		events[i] = testEvent(fmt.Sprintf("evt-%d", i), base)
	}

	return events
}

// BenchmarkStorageStoreBatch measures the write path against a live server
// across batch sizes. The interesting number is ns/row: ClickHouse pays a
// fixed cost per INSERT, so small batches spread that cost over few rows.
// This is the measurement behind the batch-size warning in
// data/audit/factory.
func BenchmarkStorageStoreBatch(b *testing.B) {
	for _, size := range []int{100, 1000, 10_000} {
		b.Run(fmt.Sprintf("rows=%d", size), func(b *testing.B) {
			s := newTestStorage(b)
			events := benchEvents(size)
			ctx := b.Context()

			b.ReportAllocs()
			b.ResetTimer()

			rows := 0
			for b.Loop() {
				if err := s.StoreBatch(ctx, events); err != nil {
					b.Fatal(err)
				}
				rows += size
			}

			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(rows), "ns/row")
		})
	}
}

// BenchmarkStorageQuery measures the read path over a preloaded table.
func BenchmarkStorageQuery(b *testing.B) {
	const preload = 20_000

	s := newTestStorage(b)
	ctx := b.Context()

	if err := s.StoreBatch(ctx, benchEvents(preload)); err != nil {
		b.Fatal(err)
	}

	for _, limit := range []int{100, 1000} {
		b.Run(fmt.Sprintf("limit=%d", limit), func(b *testing.B) {
			query := &audit.Query{ActorID: "user-42", Limit: limit}

			b.ReportAllocs()
			b.ResetTimer()

			rows := 0
			for b.Loop() {
				for _, err := range s.Query(ctx, query) {
					if err != nil {
						b.Fatal(err)
					}
					rows++
				}
			}

			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(rows), "ns/row")
		})
	}
}

// BenchmarkStorageCount measures an aggregate over the same preloaded table,
// with and without the FINAL modifier that deduplication reads require.
func BenchmarkStorageCount(b *testing.B) {
	const preload = 20_000

	events := benchEvents(preload)

	for _, final := range []bool{false, true} {
		b.Run(fmt.Sprintf("final=%t", final), func(b *testing.B) {
			opts := coreslices.AppendIf(nil, final, auditclickhouse.WithFinal())

			s := newTestStorage(b, opts...)
			ctx := b.Context()
			if err := s.StoreBatch(ctx, events); err != nil {
				b.Fatal(err)
			}

			query := &audit.Query{ActorID: "user-42"}

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				if _, err := s.Count(ctx, query); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
