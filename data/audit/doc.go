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
// Events flow through the following pipeline:
//
//	Emit() → [Channel Buffer] → Worker Pool → Batch Buffer → StoreBatch()
//	                                                              ↓
//	                                                   (fail? → Retry with backoff)
//
// # Usage
//
//	storage := memory.New() // or mongo.New(db)
//	auditor, _ := audit.New(storage,
//	    audit.WithServiceInfo(audit.ServiceInfo{Name: "my-service"}),
//	    audit.WithBufferSize(10000),
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
