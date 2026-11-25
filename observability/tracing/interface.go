// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import "context"

// Tracer creates recorders for instrumentation. Its methods are safe for
// concurrent use. After the provider shuts down, new spans become no-ops.
type Tracer interface {
	// Recorder returns a Recorder for instrumentation.
	// The name should identify the instrumentation library (e.g., "myapp/orders").
	Recorder(name string, opts ...RecorderOption) Recorder

	// WithScope returns a Tracer that prefixes recorder names with scope.
	WithScope(scope string) Tracer
}

// TracerProvider owns the export lifecycle for the root tracer and its scopes.
// [New] and [Noop] return a provider. Pass [Tracer] to instrumentation.
type TracerProvider interface {
	Tracer

	// ForceFlush exports pending spans and flushes the adapter.
	ForceFlush(ctx context.Context) error

	// Shutdown drains pending spans and shuts down the adapter.
	Shutdown(ctx context.Context) error
}

// Recorder creates [Span] instances for tracking operations.
// Obtain via [Tracer.Recorder].
type Recorder interface {
	// Start creates a new span and returns the updated context.
	// The returned context contains the span and should be used for child spans.
	//
	// Example:
	//   ctx, span := recorder.Start(ctx, "ProcessOrder")
	//   defer span.End()
	Start(ctx context.Context, name string, opts ...SpanStartOption) (context.Context, Span)
}

// Span represents a unit of work in a trace.
// A span tracks an operation with a start time, end time, and metadata.
// Use [Recorder.Start] to begin a new span.
// After [Span.End] is called, the span must not be modified.
type Span interface {
	// End completes the span, recording its end time. The default tracer
	// enqueues sampled spans for batch export and drops them if the queue is full.
	// After End is called, the span should not be modified.
	End(opts ...SpanEndOption)

	// SpanContext returns the SpanContext for this span.
	// The SpanContext contains identifiers and flags.
	SpanContext() SpanContext

	// IsRecording returns true if the span is recording events.
	// Non-recording spans should not have attributes or events added.
	IsRecording() bool

	// SetName changes the name of the span.
	SetName(name string)

	// SetStatus sets the status of the span.
	// Use StatusError for errors, StatusOK for success.
	SetStatus(code StatusCode, description string)

	// SetAttributes sets attributes on the span.
	// Attributes are key-value pairs that describe the span.
	SetAttributes(attrs ...Attribute)

	// RecordError records an error as a span event.
	// An explicit exception.message event attribute replaces the default err.Error() text.
	RecordError(err error, opts ...EventOption)

	// AddEvent adds an event to the span.
	// Events are timestamped annotations with attributes.
	AddEvent(name string, opts ...EventOption)
}

// SpanContext contains identifying information about a span.
// SpanContext is immutable and safe for concurrent use.
// It can be serialized for cross-process propagation.
type SpanContext interface {
	// TraceID returns the trace ID as a hex string.
	// Returns empty string if the span context is invalid.
	TraceID() string

	// SpanID returns the span ID as a hex string.
	// Returns empty string if the span context is invalid.
	SpanID() string

	// TraceFlags returns the trace flags.
	TraceFlags() TraceFlags

	// IsValid returns true if the span context has valid trace and span IDs.
	IsValid() bool

	// IsRemote returns true if the span context was propagated from a remote parent.
	IsRemote() bool

	// IsSampled returns true if the span is sampled (will be exported).
	IsSampled() bool
}

// StatusCode represents the status of a [Span].
// The zero value is [StatusUnset].
type StatusCode int

const (
	// StatusUnset is the default status code.
	StatusUnset StatusCode = iota

	// StatusOK indicates the operation completed successfully.
	StatusOK

	// StatusError indicates the operation contained an error.
	StatusError
)

// String returns the string representation of the status code.
func (c StatusCode) String() string {
	switch c {
	case StatusUnset:
		return "Unset"
	case StatusOK:
		return "OK"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// TraceFlags represents trace flags as defined in the W3C Trace Context specification.
// The only currently defined flag is [FlagsSampled].
type TraceFlags byte

const (
	// FlagsSampled indicates the trace is sampled.
	FlagsSampled TraceFlags = 0x01
)

// IsSampled returns true if the sampled flag is set.
func (f TraceFlags) IsSampled() bool {
	return f&FlagsSampled == FlagsSampled
}

// WithSampled returns the flags with the sampled flag set or cleared.
func (f TraceFlags) WithSampled(sampled bool) TraceFlags {
	if sampled {
		return f | FlagsSampled
	}
	return f &^ FlagsSampled
}

// String returns the hex representation of the flags.
func (f TraceFlags) String() string {
	var buf [2]byte
	buf[0] = hexChar(byte(f) >> 4)   //nolint:mnd // Hex conversion
	buf[1] = hexChar(byte(f) & 0x0f) //nolint:mnd // Hex conversion
	return string(buf[:])
}

func hexChar(b byte) byte {
	if b < 10 { //nolint:mnd // Standard decimal/hex boundary
		return '0' + b
	}
	return 'a' + b - 10 //nolint:mnd // Standard hex offset
}
