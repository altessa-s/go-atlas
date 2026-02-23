// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sampler

// SamplingDecision indicates whether a span should be sampled.
type SamplingDecision int

const (
	// Drop indicates the span should not be recorded or sampled.
	Drop SamplingDecision = iota

	// RecordOnly indicates the span should be recorded but not sampled.
	// The span will be processed locally but not exported.
	RecordOnly

	// RecordAndSample indicates the span should be recorded and sampled.
	// The span will be processed and exported.
	RecordAndSample
)

// String returns the string representation of the decision.
func (d SamplingDecision) String() string {
	switch d {
	case Drop:
		return "Drop"
	case RecordOnly:
		return "RecordOnly"
	case RecordAndSample:
		return "RecordAndSample"
	default:
		return "Unknown"
	}
}

// IsSampled returns true if the decision indicates the span should be sampled.
func (d SamplingDecision) IsSampled() bool {
	return d == RecordAndSample
}

// IsRecording returns true if the decision indicates the span should be recorded.
func (d SamplingDecision) IsRecording() bool {
	return d == RecordOnly || d == RecordAndSample
}

// SpanKind represents the role of the span in a trace.
// Duplicated here to avoid import cycles with the tracing package.
type SpanKind int

const (
	SpanKindUnspecified SpanKind = iota
	SpanKindInternal
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// SpanContext contains identifying information about a span.
// This is a simplified version to avoid import cycles.
type SpanContext struct {
	TraceID   [16]byte
	SpanID    [8]byte
	IsSampled bool
	IsRemote  bool
}

// IsValid returns true if the span context has valid trace and span IDs.
func (sc *SpanContext) IsValid() bool {
	if sc == nil {
		return false
	}
	return sc.TraceID != [16]byte{} && sc.SpanID != [8]byte{}
}

// Attribute represents a key-value pair.
// Duplicated here to avoid import cycles with the tracing package.
type Attribute struct {
	Key   string
	Value any
}

// Link represents a link to another span.
// Duplicated here to avoid import cycles with the tracing package.
type Link struct {
	TraceID    [16]byte
	SpanID     [8]byte
	Attributes []Attribute
}

// SamplingParameters contains information for making a sampling decision.
type SamplingParameters struct {
	// TraceID is the trace ID for the span.
	TraceID [16]byte

	// SpanID is the span ID for the span.
	SpanID [8]byte

	// Name is the name of the span being created.
	Name string

	// Kind is the kind of the span being created.
	Kind SpanKind

	// ParentCtx is the parent span context, if any.
	ParentCtx *SpanContext

	// HasRemote indicates if the parent context is remote.
	HasRemote bool

	// Attributes are the initial attributes for the span.
	Attributes []Attribute

	// Links are the links for the span.
	Links []Link
}

// SamplingResult contains the result of a sampling decision.
type SamplingResult struct {
	// Decision indicates whether the span should be sampled.
	Decision SamplingDecision

	// Attributes are additional attributes to add to the span.
	// These may be added by the sampler based on the decision.
	Attributes []Attribute

	// Tracestate is the updated tracestate string.
	Tracestate string
}

// Sampler decides whether a trace should be sampled.
// Implementations must be safe for concurrent use.
//
// Built-in samplers: [AlwaysOn], [AlwaysOff], [NewTraceIDRatio], [NewParentBased].
type Sampler interface {
	// ShouldSample returns a sampling decision for a span.
	ShouldSample(params SamplingParameters) SamplingResult

	// Description returns a description of the sampler.
	// This is used for debugging and logging.
	Description() string
}
