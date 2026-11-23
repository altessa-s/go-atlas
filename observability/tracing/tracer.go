// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"context"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/types/nilcheck"
	"github.com/altessa-s/go-atlas/observability/internal/shared"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

// tracer is the default implementation of Tracer.
type tracer struct {
	processor *batchProcessor
	sampler   sampler.Sampler
	resource  *adapters.Resource // pre-computed, shared across all spans
	recorders sync.Map           // map[adapters.InstrumentationScope]*recorder
}

// New creates a new [Tracer] with the given options.
// Returns [Noop] if no adapter is configured.
func New(opts ...Option) Tracer {
	cfg := newOptions(opts...)

	// If no adapter configured, return noop tracer
	if nilcheck.IsNil(cfg.adapter) {
		return Noop()
	}
	if nilcheck.IsNil(cfg.sampler) {
		cfg.sampler = sampler.NewParentBased(sampler.AlwaysOn())
	}

	return &tracer{
		processor: newBatchProcessor(cfg),
		sampler:   cfg.sampler,
		resource: &adapters.Resource{
			Attributes: []adapters.Attribute{
				{Key: "service.name", Value: cfg.serviceName},
				{Key: "service.version", Value: cfg.serviceVersion},
				{Key: "deployment.environment", Value: cfg.environment},
			},
		},
	}
}

// Recorder implements Tracer.
func (t *tracer) Recorder(name string, opts ...RecorderOption) Recorder {
	return t.getOrCreateRecorder("", name, opts...)
}

// Shutdown implements Tracer.
func (t *tracer) Shutdown(ctx context.Context) error {
	return t.processor.shutdown(ctx)
}

// ForceFlush implements Tracer.
func (t *tracer) ForceFlush(ctx context.Context) error {
	return t.processor.forceFlush(ctx)
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
	if t.processor.stopped() {
		return noopRecorder{}
	}
	id := adapters.InstrumentationScope{Name: shared.BuildTracerName(scope, name)}
	if len(opts) > 0 {
		cfg := ApplyRecorderOptions(opts...)
		id.Version = cfg.InstrumentationVersion()
		id.SchemaURL = cfg.SchemaURL()
	}
	return shared.GetOrCreate(&t.recorders, id, func() *recorder {
		return &recorder{tracer: t, scope: id}
	})
}

// recorder is the default implementation of Recorder.
type recorder struct {
	tracer *tracer
	scope  adapters.InstrumentationScope
}

// Start implements Recorder.
func (r *recorder) Start(ctx context.Context, spanName string, opts ...SpanStartOption) (context.Context, Span) {
	if r.tracer.processor.stopped() {
		return ctx, noopSpan{}
	}

	var cfg SpanStartConfig
	if len(opts) > 0 {
		cfg = *ApplySpanStartOptions(opts...)
	}

	// Get parent span context; SpanFromContext falls back to a no-op span.
	parent, state := snapshotParent(SpanFromContext(ctx).SpanContext())
	spanCtx := makeSpanContext(parent, state)
	var parentID [8]byte
	if parent != nil {
		parentID = parent.SpanID
	}

	// Check sampling decision
	shouldSample := r.tracer.sampler.ShouldSample(sampler.SamplingParameters{
		TraceID:   spanCtx.traceID,
		SpanID:    spanCtx.spanID,
		Name:      spanName,
		Kind:      sampler.SpanKind(cfg.Kind()),
		ParentCtx: parent,
		HasRemote: parent != nil && parent.IsRemote,
	})

	if !shouldSample.Decision.IsSampled() {
		spanCtx.traceFlags = 0
	}
	if shouldSample.Tracestate != "" {
		spanCtx.traceState = shouldSample.Tracestate
	}
	if !shouldSample.Decision.IsRecording() {
		contextCopy := spanCtx
		span := nonRecordingSpan{spanContext: &contextCopy}
		return ContextWithSpan(ctx, span), span
	}

	// Create recording span
	span := &recordingSpan{
		recorder:    r,
		parentID:    parentID,
		name:        spanName,
		spanContext: spanCtx,
		kind:        cfg.Kind(),
		startTime:   cfg.Timestamp(),
		attributes:  cfg.Attributes(),
		links:       cfg.Links(),
	}
	if span.startTime.IsZero() {
		span.startTime = time.Now()
	}
	return ContextWithSpan(ctx, span), span
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
