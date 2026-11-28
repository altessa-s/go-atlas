// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

//go:generate go tool optgen generate --type=recorderOptions --option-type=RecorderOption --string-setter=plain --keep-empty-strings --skip-default-func --skip-new-func --output=recorder_options_gen.go
//go:generate go tool optgen generate --type=spanStartOptions --option-type=SpanStartOption --skip-default-func --skip-new-func --output=span_start_options_gen.go
//go:generate go tool optgen generate --type=spanEndOptions --option-type=SpanEndOption --skip-default-func --skip-new-func --output=span_end_options_gen.go
//go:generate go tool optgen generate --type=eventOptions --option-type=EventOption --skip-default-func --skip-new-func --output=event_options_gen.go

import (
	"iter"
	"slices"
	"time"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
)

// recorderOptions holds the settings of a [Recorder] obtained from
// [Tracer.Recorder]; [RecorderConfig] is their resolved, read-only form.
type recorderOptions struct {
	// WithInstrumentationVersion sets the instrumentation version for the recorder.
	instrumentationVersion string `optval:"notrim"`
	// WithSchemaURL sets the schema URL for the recorder.
	schemaURL string `optval:"notrim"`
}

// RecorderConfig is the resolved set of [RecorderOption] values, returned by
// [ApplyRecorderOptions]. It is read-only: a [Tracer] implementation reads it
// through its accessors.
type RecorderConfig struct {
	recorderOptions
}

// spanStartOptions holds the settings of a span being started;
// [SpanStartConfig] is their resolved, read-only form.
type spanStartOptions struct {
	// WithSpanKind sets the span kind.
	kind SpanKind `opt:"SpanKind" optval:"nonpositive"`
	// WithAttributes sets initial attributes on the span. Repeated options
	// accumulate.
	attributes []Attribute `optgen:"append" optval:"nodup"`
	// WithStartTimestamp sets the start time of the span.
	timestamp time.Time `opt:"StartTimestamp"`
	// WithLinks adds links to other spans. Repeated options accumulate.
	links []Link `optgen:"append" optval:"nodup"`
	// WithSampledParent records the span only under a sampled parent span. Without one, Start returns the context
	// unchanged and a span that records nothing, so work outside a sampled trace starts no trace of its own.
	sampledParent bool
}

// SpanStartConfig is the resolved set of [SpanStartOption] values, returned by
// [ApplySpanStartOptions]. It is read-only: a [Recorder] implementation reads
// it through its accessors.
type SpanStartConfig struct {
	spanStartOptions
}

// spanEndOptions holds the settings of a span being ended; [SpanEndConfig] is
// their resolved, read-only form.
type spanEndOptions struct {
	// WithEndTimestamp sets the end time of the span.
	timestamp time.Time `opt:"EndTimestamp"`
}

// SpanEndConfig is the resolved set of [SpanEndOption] values, returned by
// [ApplySpanEndOptions]. It is read-only: a [Span] implementation reads it
// through its accessors.
type SpanEndConfig struct {
	spanEndOptions
}

// eventOptions holds the settings of a span event; [EventConfig] is their
// resolved, read-only form.
type eventOptions struct {
	// WithEventTimestamp sets the timestamp for an event.
	timestamp time.Time `opt:"EventTimestamp"`
	// WithEventAttributes sets attributes for an event. Repeated options
	// accumulate.
	attributes []Attribute `opt:"EventAttributes" optgen:"append" optval:"nodup"`
	// WithStackTrace enables or disables stack trace recording for an event.
	stackTrace bool `optval:"param"`
}

// EventConfig is the resolved set of [EventOption] values, returned by
// [ApplyEventOptions]. It is read-only: a [Span] implementation reads it
// through its accessors.
type EventConfig struct {
	eventOptions
}

// Link represents a link to another span.
type Link struct {
	SpanContext SpanContext
	Attributes  []Attribute
}

// linksPool provides pooling for Link slices.
var linksPool = coreslices.NewPool[Link](4) //nolint:mnd // Pool capacity hint

// --- Config accessors for adapters ---

// ApplyRecorderOptions applies options and returns the config.
func ApplyRecorderOptions(opts ...RecorderOption) *RecorderConfig {
	c := &RecorderConfig{}
	for _, opt := range opts {
		opt(&c.recorderOptions)
	}
	return c
}

// ApplySpanStartOptions applies options and returns the config.
func ApplySpanStartOptions(opts ...SpanStartOption) *SpanStartConfig {
	c := &SpanStartConfig{}
	for _, opt := range opts {
		opt(&c.spanStartOptions)
	}
	return c
}

// ApplySpanEndOptions applies options and returns the config.
func ApplySpanEndOptions(opts ...SpanEndOption) *SpanEndConfig {
	c := &SpanEndConfig{}
	for _, opt := range opts {
		opt(&c.spanEndOptions)
	}
	return c
}

// ApplyEventOptions applies options and returns the config.
func ApplyEventOptions(opts ...EventOption) *EventConfig {
	c := &EventConfig{}
	for _, opt := range opts {
		opt(&c.eventOptions)
	}
	return c
}

// --- RecorderConfig getters ---

// InstrumentationVersion returns the instrumentation version.
func (c *RecorderConfig) InstrumentationVersion() string { return c.instrumentationVersion }

// SchemaURL returns the schema URL.
func (c *RecorderConfig) SchemaURL() string { return c.schemaURL }

// --- SpanStartConfig getters ---

// Kind returns the span kind.
func (c *SpanStartConfig) Kind() SpanKind { return c.kind }

// Attributes returns the initial attributes.
func (c *SpanStartConfig) Attributes() []Attribute { return c.attributes }

// Timestamp returns the start timestamp.
func (c *SpanStartConfig) Timestamp() time.Time { return c.timestamp }

// Links returns the span links.
func (c *SpanStartConfig) Links() []Link { return c.links }

// SampledParent reports whether the span records only under a sampled parent span ([WithSampledParent]).
func (c *SpanStartConfig) SampledParent() bool { return c.sampledParent }

// LinksIter returns an iterator over the span links.
func (c *SpanStartConfig) LinksIter() iter.Seq[Link] {
	return slices.Values(c.links)
}

// AttributesIter returns an iterator over the attributes.
func (c *SpanStartConfig) AttributesIter() iter.Seq[Attribute] {
	return slices.Values(c.attributes)
}

// --- SpanEndConfig getters ---

// Timestamp returns the end timestamp.
func (c *SpanEndConfig) Timestamp() time.Time { return c.timestamp }

// --- EventConfig getters ---

// Timestamp returns the event timestamp.
func (c *EventConfig) Timestamp() time.Time { return c.timestamp }

// Attributes returns the event attributes.
func (c *EventConfig) Attributes() []Attribute { return c.attributes }

// AttributesIter returns an iterator over the event attributes.
func (c *EventConfig) AttributesIter() iter.Seq[Attribute] {
	return slices.Values(c.attributes)
}

// StackTrace returns whether stack trace is enabled.
func (c *EventConfig) StackTrace() bool { return c.stackTrace }

// --- Link pooling ---

// GetLinks retrieves a Link slice from the pool.
func GetLinks() *[]Link {
	return linksPool.Get()
}

// GetLinksWithCapacity retrieves a Link slice from the pool with the given capacity.
func GetLinksWithCapacity(capacity int) *[]Link {
	return linksPool.GetWithCapacity(capacity)
}

// PutLinks returns a Link slice to the pool for reuse.
func PutLinks(links *[]Link) {
	linksPool.Put(links)
}
