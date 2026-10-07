// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/keyset"
)

// sliceStorage yields a fixed slice of events up to the query limit, so the
// benchmarks measure FetchPage itself rather than a storage.
type sliceStorage struct {
	audit.Storage
	events []*audit.Event
}

func (s sliceStorage) Query(_ context.Context, q *audit.Query) iter.Seq2[*audit.Event, error] {
	return func(yield func(*audit.Event, error) bool) {
		for _, e := range s.events[:min(q.Limit, len(s.events))] {
			if !yield(e, nil) {
				return
			}
		}
	}
}

func benchTokens(b *testing.B) *audit.PageTokens {
	b.Helper()
	codec, err := keyset.New(bytes.Repeat([]byte{7}, keyset.MinKeyLength))
	if err != nil {
		b.Fatal(err)
	}
	return audit.NewPageTokens(codec)
}

func benchEvents(n int) []*audit.Event {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	events := make([]*audit.Event, n)
	for i := range events {
		events[i] = &audit.Event{ID: fmt.Sprintf("evt-%06d", i), Timestamp: base.Add(time.Duration(i) * time.Millisecond)}
	}
	return events
}

func BenchmarkFetchPage(b *testing.B) {
	for _, size := range []int{audit.DefaultPageSize, 1000} {
		b.Run(fmt.Sprintf("limit=%d", size), func(b *testing.B) {
			storage := sliceStorage{events: benchEvents(size + 1)}
			tokens := benchTokens(b)
			ctx := b.Context()
			query := audit.Query{Limit: size, ActorID: "user-1"}

			b.ReportAllocs()
			for b.Loop() {
				if _, err := audit.FetchPage(ctx, storage, tokens, query); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkPageTokens_Encode(b *testing.B) {
	tokens := benchTokens(b)
	event := benchEvents(1)[0]
	query := &audit.Query{ActorID: "user-1", ResourceType: "doc"}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := tokens.Encode(event, query); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPageTokens_Decode(b *testing.B) {
	tokens := benchTokens(b)
	query := &audit.Query{ActorID: "user-1", ResourceType: "doc"}
	token, err := tokens.Encode(benchEvents(1)[0], query)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := tokens.Decode(token, query); err != nil {
			b.Fatal(err)
		}
	}
}
