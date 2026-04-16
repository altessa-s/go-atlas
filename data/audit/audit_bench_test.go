// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/service/dispatch"
	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
)

func BenchmarkAuditor_Emit(b *testing.B) {
	store := memory.New()
	eng := newTestEngine(b, store,
		dispatch.WithBufferSize[*audit.Event](100000),
		dispatch.WithBatchSize[*audit.Event](500),
		dispatch.WithFlushInterval[*audit.Event](50*time.Millisecond),
		dispatch.WithWorkers[*audit.Event](4),
	)
	if err := eng.Start(); err != nil {
		b.Fatal(err)
	}
	a, err := audit.New(eng)
	if err != nil {
		b.Fatal(err)
	}
	if err := a.Start(); err != nil {
		b.Fatal(err)
	}
	defer func() {
		a.Shutdown(b.Context())
		eng.Shutdown(b.Context())
	}()

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
	eng := newTestEngine(b, store)
	if err := eng.Start(); err != nil {
		b.Fatal(err)
	}
	defer eng.Shutdown(b.Context())
	a, err := audit.New(eng)
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
		eng := newTestEngine(b, store)
		_ = eng.Start()
		_, _ = audit.New(eng)
		eng.Shutdown(b.Context())
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
