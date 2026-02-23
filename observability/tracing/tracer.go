// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/observability/internal/shared"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

// tracer is the default implementation of Tracer.
type tracer struct {
	serviceName    string
	serviceVersion string
	environment    string
	adapter        adapters.Adapter
	sampler        sampler.Sampler
	recorders      sync.Map // map[string]*recorder
	shutdown       atomic.Bool
}

// New creates a new [Tracer] with the given options.
// Returns [Noop] if no adapter is configured.
func New(opts ...Option) Tracer {
	cfg := newOptions(opts...)

	// If no adapter configured, return noop tracer
	if cfg.adapter == nil {
		return Noop()
	}

	return &tracer{
		serviceName:    cfg.serviceName,
		serviceVersion: cfg.serviceVersion,
		environment:    cfg.environment,
		adapter:        cfg.adapter,
		sampler:        cfg.sampler,
	}
}

// Recorder implements Tracer.
func (t *tracer) Recorder(name string, opts ...RecorderOption) Recorder {
	if t.shutdown.Load() {
		return noopRecorder{}
	}
	return t.getOrCreateRecorder("", name, opts...)
}

// Shutdown implements Tracer.
func (t *tracer) Shutdown(ctx context.Context) error {
	if t.shutdown.Load() {
		return nil // Already shutdown
	}

	// Flush first (before setting shutdown flag)
	if err := t.adapter.ForceFlush(ctx); err != nil {
		return err
	}

	// Mark as shutdown
	if !t.shutdown.CompareAndSwap(false, true) {
		return nil // Already shutdown (race condition)
	}

	// Shutdown adapter
	return t.adapter.Shutdown(ctx)
}

// ForceFlush implements Tracer.
func (t *tracer) ForceFlush(ctx context.Context) error {
	if t.shutdown.Load() {
		return ErrTracerShutdown
	}
	return t.adapter.ForceFlush(ctx)
}

// WithScope implements Tracer.
func (t *tracer) WithScope(scope string) Tracer {
	return &scopedTracer{
		parent: t,
		scope:  scope,
	}
}

// getOrCreateRecorder retrieves or creates a recorder with the given name.
func (t *tracer) getOrCreateRecorder(scope, name string, opts ...RecorderOption) Recorder {
	fullName := shared.BuildTracerName(scope, name)

	return shared.GetOrCreate(&t.recorders, fullName, func() *recorder {
		return newRecorder(t, fullName, opts...)
	})
}

// export exports a completed span through the adapter.
// Export errors are intentionally ignored to avoid blocking the hot path.
func (t *tracer) export(ctx context.Context, span *adapters.SpanData) {
	if t.shutdown.Load() {
		return
	}
	_ = t.adapter.ExportSpans(ctx, []adapters.SpanData{*span}) //nolint:errcheck
}

// recorder is the default implementation of Recorder.
type recorder struct {
	tracer *tracer
	name   string
	config *RecorderConfig
}

// newRecorder creates a new recorder.
func newRecorder(t *tracer, name string, opts ...RecorderOption) *recorder {
	return &recorder{
		tracer: t,
		name:   name,
		config: ApplyRecorderOptions(opts...),
	}
}

// Start implements Recorder.
func (r *recorder) Start(ctx context.Context, spanName string, opts ...SpanStartOption) (context.Context, Span) {
	if r.tracer.shutdown.Load() {
		return ctx, noopSpan{}
	}

	cfg := ApplySpanStartOptions(opts...)

	// Get parent span context if exists
	parentSpan := SpanFromContext(ctx)
	var parentSpanContext SpanContext
	if parentSpan != nil {
		parentSpanContext = parentSpan.SpanContext()
	}

	// Create span context
	spanCtx := newSpanContextImpl(parentSpanContext)

	// Check sampling decision
	shouldSample := r.tracer.sampler.ShouldSample(sampler.SamplingParameters{
		TraceID:   spanCtx.traceID,
		SpanID:    spanCtx.spanID,
		Name:      spanName,
		Kind:      sampler.SpanKind(cfg.Kind()),
		ParentCtx: extractSamplerSpanContext(parentSpanContext),
		HasRemote: parentSpanContext != nil && parentSpanContext.IsRemote(),
	})

	if !shouldSample.Decision.IsSampled() {
		// Return non-recording span
		return ContextWithSpan(ctx, noopSpan{}), noopSpan{}
	}

	// Create recording span
	span := &recordingSpan{
		recorder:    r,
		name:        spanName,
		spanContext: spanCtx,
		kind:        cfg.Kind(),
		startTime:   cfg.Timestamp(),
		attributes:  CloneAttributes(cfg.Attributes()),
		links:       slices.Clone(cfg.Links()),
	}

	if span.startTime.IsZero() {
		span.startTime = time.Now()
	}

	// Set parent ID if we have a parent
	if parentSpanContext != nil && parentSpanContext.IsValid() {
		span.parentID = parseSpanID(parentSpanContext.SpanID())
	}

	return ContextWithSpan(ctx, span), span
}

// extractSamplerSpanContext converts SpanContext interface to sampler.SpanContext.
func extractSamplerSpanContext(sc SpanContext) *sampler.SpanContext {
	if sc == nil || !sc.IsValid() {
		return nil
	}
	return &sampler.SpanContext{
		TraceID:   parseTraceID(sc.TraceID()),
		SpanID:    parseSpanID(sc.SpanID()),
		IsSampled: sc.IsSampled(),
		IsRemote:  sc.IsRemote(),
	}
}

// Hex ID sizes.
const (
	traceIDHexLen = 32 // 16 bytes * 2 hex chars
	spanIDHexLen  = 16 // 8 bytes * 2 hex chars
)

// parseTraceID parses a hex trace ID string into bytes.
func parseTraceID(s string) [16]byte {
	var id [16]byte
	if len(s) != traceIDHexLen {
		return id
	}
	for i := range 16 {
		id[i] = hexToByte(s[i*2], s[i*2+1])
	}
	return id
}

// parseSpanID parses a hex span ID string into bytes.
func parseSpanID(s string) [8]byte {
	var id [8]byte
	if len(s) != spanIDHexLen {
		return id
	}
	for i := range 8 {
		id[i] = hexToByte(s[i*2], s[i*2+1])
	}
	return id
}

// hexToByte converts two hex characters to a byte.
func hexToByte(hi, lo byte) byte {
	return hexVal(hi)<<4 | hexVal(lo)
}

// hexVal converts a hex character to its value.
func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10 //nolint:mnd // Standard hex offset
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10 //nolint:mnd // Standard hex offset
	default:
		return 0
	}
}

// Ensure tracer implements Tracer.
var _ Tracer = (*tracer)(nil)

// Ensure recorder implements Recorder.
var _ Recorder = (*recorder)(nil)
