// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package audit provides high-performance, asynchronous user action auditing
// with at-least-once delivery semantics.
//
// The package uses an async dispatcher with channel buffering, batch writes,
// and retry with exponential backoff. It has zero impact on request latency
// since all event emission is non-blocking.
//
// # Architecture
//
// Auditor is a thin facade over a [Dispatcher] (typically [dispatch.Engine]).
// The caller creates and starts the dispatch engine, then passes it to [New].
// Events flow through the engine's pipeline:
//
//	Emit() → Dispatcher.Submit() → [Channel Buffer] → Worker Pool → Batch → StoreBatch()
//	                                                                              ↓
//	                                                                   (fail? → Retry with backoff)
//
// # Usage
//
//	storage := memory.New() // or mongo.New(db)
//	eng, _ := dispatch.NewEngine[*audit.Event](
//	    audit.StorageSink{Storage: storage},
//	    dispatch.WithBufferSize[*audit.Event](10000),
//	)
//	eng.Start()
//	defer eng.Shutdown(ctx)
//
//	auditor, _ := audit.New(eng,
//	    audit.WithServiceInfo(audit.ServiceInfo{Name: "my-service"}),
//	)
//	auditor.Start()
//	defer auditor.Shutdown(ctx)
//
//	// Fire-and-forget
//	auditor.Emit(&audit.Event{...})
//
//	// Fluent builder
//	auditor.NewEvent(audit.EventTypeDataChange, audit.ActionUpdate).
//	    WithResource(resource).
//	    WithSuccess().
//	    Emit()
package audit
