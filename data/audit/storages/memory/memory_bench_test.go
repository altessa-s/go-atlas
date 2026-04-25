// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
)

func BenchmarkStorage_Store(b *testing.B) {
	s := memory.New()
	ctx := b.Context()
	event := &audit.Event{
		Type:   audit.EventTypeSystem,
		Action: audit.ActionExecute,
	}

	b.ResetTimer()
	for b.Loop() {
		_ = s.Store(ctx, event)
	}
}

func BenchmarkStorage_StoreBatch(b *testing.B) {
	s := memory.New()
	ctx := b.Context()
	events := make([]*audit.Event, 100)
	for i := range events {
		events[i] = &audit.Event{
			Type:   audit.EventTypeSystem,
			Action: audit.ActionExecute,
			Actor:  audit.Actor{ID: string(rune('0' + i%10))},
		}
	}

	b.ResetTimer()
	for b.Loop() {
		_ = s.StoreBatch(ctx, events)
	}
}

func BenchmarkStorage_Query(b *testing.B) {
	s := memory.New()
	ctx := b.Context()

	for range 1000 {
		_ = s.Store(ctx, &audit.Event{
			Type:      audit.EventTypeAPIRequest,
			Action:    audit.ActionRead,
			Actor:     audit.Actor{Type: audit.ActorTypeUser, ID: "u1"},
			Timestamp: time.Now(),
		})
	}

	query := &audit.Query{ActorID: "u1", Limit: 10}
	b.ResetTimer()
	for b.Loop() {
		for range s.Query(ctx, query) {
		}
	}
}

func BenchmarkStorage_Count(b *testing.B) {
	s := memory.New()
	ctx := b.Context()

	for range 1000 {
		_ = s.Store(ctx, &audit.Event{
			Actor:     audit.Actor{ID: "u1"},
			Timestamp: time.Now(),
		})
	}

	query := &audit.Query{ActorID: "u1"}
	b.ResetTimer()
	for b.Loop() {
		_, _ = s.Count(ctx, query)
	}
}
