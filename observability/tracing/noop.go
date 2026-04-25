// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import "context"

// noopTracer is a no-op implementation of Tracer.
// All operations are safe but do nothing.
type noopTracer struct{}

var noopTracerInstance = &noopTracer{}

// Noop returns a no-op implementation of [Tracer].
// All operations are safe but nothing is recorded or exported.
// The returned value is a singleton; [IsNoop] can identify it.
//
// Use Noop() as the default when tracing is optional:
//
//	func newOptions(opts ...Option) *options {
//	    o := &options{
//	        tracer: tracing.Noop(), // Default to no-op
//	    }
//	    // ...
//	}
func Noop() Tracer {
	return noopTracerInstance
}

// IsNoop reports whether t is the singleton no-op [Tracer] returned by [Noop].
func IsNoop(t Tracer) bool {
	return t == noopTracerInstance
}

// Recorder implements Tracer.
func (n *noopTracer) Recorder(_ string, _ ...RecorderOption) Recorder {
	return noopRecorder{}
}

// Shutdown implements Tracer.
func (n *noopTracer) Shutdown(_ context.Context) error {
	return nil
}

// ForceFlush implements Tracer.
func (n *noopTracer) ForceFlush(_ context.Context) error {
	return nil
}

// WithScope implements Tracer.
func (n *noopTracer) WithScope(_ string) Tracer {
	return n
}

// noopRecorder is a no-op implementation of Recorder.
type noopRecorder struct{}

// Start implements Recorder.
func (noopRecorder) Start(ctx context.Context, _ string, _ ...SpanStartOption) (context.Context, Span) {
	return ctx, noopSpan{}
}

// noopSpan is a no-op implementation of Span.
type noopSpan struct{}

// End implements Span.
func (noopSpan) End(_ ...SpanEndOption) {}

// SpanContext implements Span.
func (noopSpan) SpanContext() SpanContext {
	return noopSpanContext{}
}

// IsRecording implements Span.
func (noopSpan) IsRecording() bool {
	return false
}

// SetName implements Span.
func (noopSpan) SetName(_ string) {}

// SetStatus implements Span.
func (noopSpan) SetStatus(_ StatusCode, _ string) {}

// SetAttributes implements Span.
func (noopSpan) SetAttributes(_ ...Attribute) {}

// RecordError implements Span.
func (noopSpan) RecordError(_ error, _ ...EventOption) {}

// AddEvent implements Span.
func (noopSpan) AddEvent(_ string, _ ...EventOption) {}

// noopSpanContext is a no-op implementation of SpanContext.
type noopSpanContext struct{}

// TraceID implements SpanContext.
func (noopSpanContext) TraceID() string {
	return ""
}

// SpanID implements SpanContext.
func (noopSpanContext) SpanID() string {
	return ""
}

// TraceFlags implements SpanContext.
func (noopSpanContext) TraceFlags() TraceFlags {
	return 0
}

// IsValid implements SpanContext.
func (noopSpanContext) IsValid() bool {
	return false
}

// IsRemote implements SpanContext.
func (noopSpanContext) IsRemote() bool {
	return false
}

// IsSampled implements SpanContext.
func (noopSpanContext) IsSampled() bool {
	return false
}
