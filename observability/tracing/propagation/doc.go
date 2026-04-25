// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package propagation provides context propagation for distributed tracing.
// It implements W3C Trace Context and Baggage specifications for extracting
// and injecting trace context across service boundaries.
//
// # W3C Trace Context
//
// The package supports W3C Trace Context headers:
//   - traceparent: Contains trace ID, span ID, and trace flags
//   - tracestate: Optional vendor-specific trace data
//
// Example:
//
//	propagator := propagation.NewTraceContext()
//
//	// Extract from incoming request
//	ctx := propagator.Extract(ctx, propagation.HeaderCarrier(req.Header))
//
//	// Inject into outgoing request
//	propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))
//
// # TextMapPropagator Interface
//
// The TextMapPropagator interface allows custom propagation formats:
//
//	type MyPropagator struct{}
//
//	func (p *MyPropagator) Inject(ctx context.Context, carrier TextMapCarrier)
//	func (p *MyPropagator) Extract(ctx context.Context, carrier TextMapCarrier) context.Context
//	func (p *MyPropagator) Fields() []string
package propagation
