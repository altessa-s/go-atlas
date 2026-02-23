// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tracing provides an abstract distributed tracing system that is not tied
// to any specific format (Jaeger, Zipkin, OpenTelemetry, etc.). Components use
// the abstract API, and export happens through adapters.
//
// # Architecture
//
// The package follows an adapter pattern similar to observability/metrics:
//   - Components depend on abstract interfaces (TracerProvider, Tracer, Span)
//   - TracerProvider manages tracer creation and lifecycle (like metrics.Collector)
//   - Adapters translate abstract spans to specific backends (OTLP, Console, etc.)
//   - Samplers control which spans are recorded and exported
//
// # Basic Usage
//
//	// Create a tracer provider with OTLP adapter
//	provider := tracing.New(
//	    tracing.WithServiceName("myapp"),
//	    tracing.WithServiceVersion("1.0.0"),
//	    tracing.WithAdapter(otlpAdapter),
//	    tracing.WithSampler(sampler.NewParentBased(sampler.NewTraceIDRatio(0.1))),
//	)
//	defer provider.Shutdown(context.Background())
//
//	// Get a tracer
//	tracer := provider.Tracer("myapp/orders")
//
//	// Start a span
//	ctx, span := tracer.Start(ctx, "ProcessOrder",
//	    tracing.WithSpanKind(tracing.SpanKindServer),
//	)
//	defer span.End()
//
//	// Add attributes
//	span.SetAttributes(
//	    tracing.String("order.id", orderID),
//	    tracing.Int("order.items", itemCount),
//	)
//
// # Factory-based Creation
//
// For configuration-driven setup, use the factory package:
//
//	f := factory.New(factory.WithLogger(logger))
//	provider, err := f.CreateFromConfig(ctx, &cfg.Observability.Tracing)
//	if err != nil {
//	    return err
//	}
//	defer provider.Shutdown(ctx)
//
// # Scoped Providers
//
// Use WithScope for organizing tracers by subsystem (similar to metrics.Collector.WithSubsystem):
//
//	ordersTracing := provider.WithScope("orders")
//	tracer := ordersTracing.Tracer("handler") // creates "orders/handler" tracer
//
// # Context Propagation
//
// Spans are propagated through context.Context:
//
//	// Get the current span from context
//	span := tracing.SpanFromContext(ctx)
//
//	// Get trace ID for logging correlation
//	traceID := tracing.TraceIDFromContext(ctx)
//	logger.Info("processing request", slog.String("trace_id", traceID))
//
// # Optional Tracing
//
// Components should accept tracing.TracerProvider through options and default to Noop():
//
//	type options struct {
//	    tracerProvider tracing.TracerProvider
//	}
//
//	func newOptions(opts ...Option) *options {
//	    o := &options{
//	        tracerProvider: tracing.Noop(), // Default to no-op
//	    }
//	    for _, opt := range opts {
//	        opt(o)
//	    }
//	    return o
//	}
//
// # Error Recording
//
// Record errors and set span status:
//
//	if err != nil {
//	    span.RecordError(err)
//	    span.SetStatus(tracing.StatusError, err.Error())
//	    return err
//	}
//	span.SetStatus(tracing.StatusOK, "")
package tracing
