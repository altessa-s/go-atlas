// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
)

func BenchmarkAuditor_Emit(b *testing.B) {
	store := memory.New()
	a, err := audit.New(store,
		audit.WithBufferSize(100000),
		audit.WithBatchSize(500),
		audit.WithFlushInterval(50*time.Millisecond),
		audit.WithWorkers(4),
	)
	if err != nil {
		b.Fatal(err)
	}
	if err := a.Start(); err != nil {
		b.Fatal(err)
	}
	defer a.Shutdown(b.Context())

	event := &audit.Event{
		Type:   audit.EventTypeAPIRequest,
		Action: audit.ActionRead,
		Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "u1"},
		Resource: audit.Resource{
			Type: "endpoint",
			ID:   "/api/v1/items",
		},
		Result: audit.Result{Status: audit.ResultStatusSuccess},
	}

	b.ResetTimer()
	for b.Loop() {
		a.Emit(event)
	}
}

func BenchmarkAuditor_NewEvent_Build(b *testing.B) {
	store := memory.New()
	a, err := audit.New(store)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for b.Loop() {
		a.NewEvent(audit.EventTypeDataChange, audit.ActionUpdate).
			WithActor(audit.Actor{Type: audit.ActorTypeUser, ID: "u1"}).
			WithResource(audit.Resource{Type: "doc", ID: "d1"}).
			WithSuccess().
			Build()
	}
}

func BenchmarkAuditor_New(b *testing.B) {
	store := memory.New()
	b.ResetTimer()
	for b.Loop() {
		_, _ = audit.New(store,
			audit.WithBufferSize(1000),
			audit.WithBatchSize(50),
			audit.WithWorkers(2),
		)
	}
}

func BenchmarkMemoryStorage_Store(b *testing.B) {
	store := memory.New()
	ctx := b.Context()
	event := &audit.Event{
		Type:   audit.EventTypeSystem,
		Action: audit.ActionExecute,
		Actor:  audit.Actor{Type: audit.ActorTypeSystem, ID: "sys"},
	}

	b.ResetTimer()
	for b.Loop() {
		_ = store.Store(ctx, event)
	}
}

func BenchmarkMemoryStorage_Query(b *testing.B) {
	store := memory.New()
	ctx := b.Context()

	for range 1000 {
		_ = store.Store(ctx, &audit.Event{
			Type:   audit.EventTypeAPIRequest,
			Action: audit.ActionRead,
			Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "u1"},
			Result: audit.Result{Status: audit.ResultStatusSuccess},
		})
	}

	query := &audit.Query{ActorID: "u1", Limit: 10}
	b.ResetTimer()
	for b.Loop() {
		for range store.Query(ctx, query) {
		}
	}
}

func BenchmarkMemoryStorage_Count(b *testing.B) {
	store := memory.New()
	ctx := b.Context()

	for range 1000 {
		_ = store.Store(ctx, &audit.Event{
			Type:   audit.EventTypeAPIRequest,
			Action: audit.ActionRead,
			Actor:  audit.Actor{Type: audit.ActorTypeUser, ID: "u1"},
		})
	}

	query := &audit.Query{ActorID: "u1"}
	b.ResetTimer()
	for b.Loop() {
		_, _ = store.Count(ctx, query)
	}
}
