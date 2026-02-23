// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"iter"
	"time"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// RecorderOption configures a Recorder.
type RecorderOption interface {
	applyRecorder(*RecorderConfig)
}

// RecorderConfig holds configuration for a Recorder.
// Exported for use by adapters.
type RecorderConfig struct {
	instrumentationVersion string
	schemaURL              string
}

// SpanStartOption configures span creation.
type SpanStartOption interface {
	applySpanStart(*SpanStartConfig)
}

// SpanStartConfig holds configuration for starting a span.
// Exported for use by adapters.
type SpanStartConfig struct {
	kind       SpanKind
	attributes []Attribute
	timestamp  time.Time
	links      []Link
}

// SpanEndOption configures span completion.
type SpanEndOption interface {
	applySpanEnd(*SpanEndConfig)
}

// SpanEndConfig holds configuration for ending a span.
// Exported for use by adapters.
type SpanEndConfig struct {
	timestamp time.Time
}

// EventOption configures span events.
type EventOption interface {
	applyEvent(*EventConfig)
}

// EventConfig holds configuration for a span event.
// Exported for use by adapters.
type EventConfig struct {
	timestamp  time.Time
	attributes []Attribute
	stackTrace bool
}

// Link represents a link to another span.
type Link struct {
	SpanContext SpanContext
	Attributes  []Attribute
}

// linksPool provides pooling for Link slices.
var linksPool = coreslices.NewPool[Link](4) //nolint:mnd // Pool capacity hint

// --- RecorderOption implementations ---

type recorderOptionFunc func(*RecorderConfig)

func (f recorderOptionFunc) applyRecorder(c *RecorderConfig) { f(c) }

// WithInstrumentationVersion sets the instrumentation version for the recorder.
func WithInstrumentationVersion(version string) RecorderOption {
	return recorderOptionFunc(func(c *RecorderConfig) {
		c.instrumentationVersion = version
	})
}

// WithSchemaURL sets the schema URL for the recorder.
func WithSchemaURL(schemaURL string) RecorderOption {
	return recorderOptionFunc(func(c *RecorderConfig) {
		c.schemaURL = schemaURL
	})
}

// --- SpanStartOption implementations ---

type spanStartOptionFunc func(*SpanStartConfig)

func (f spanStartOptionFunc) applySpanStart(c *SpanStartConfig) { f(c) }

// WithSpanKind sets the span kind.
func WithSpanKind(kind SpanKind) SpanStartOption {
	return spanStartOptionFunc(func(c *SpanStartConfig) {
		c.kind = kind
	})
}

// WithAttributes sets initial attributes on the span.
func WithAttributes(attrs ...Attribute) SpanStartOption {
	return spanStartOptionFunc(func(c *SpanStartConfig) {
		c.attributes = append(c.attributes, attrs...)
	})
}

// WithStartTimestamp sets the start time of the span.
func WithStartTimestamp(t time.Time) SpanStartOption {
	return spanStartOptionFunc(func(c *SpanStartConfig) {
		c.timestamp = t
	})
}

// WithLinks adds links to other spans.
func WithLinks(links ...Link) SpanStartOption {
	return spanStartOptionFunc(func(c *SpanStartConfig) {
		c.links = append(c.links, links...)
	})
}

// --- SpanEndOption implementations ---

type spanEndOptionFunc func(*SpanEndConfig)

func (f spanEndOptionFunc) applySpanEnd(c *SpanEndConfig) { f(c) }

// WithEndTimestamp sets the end time of the span.
func WithEndTimestamp(t time.Time) SpanEndOption {
	return spanEndOptionFunc(func(c *SpanEndConfig) {
		c.timestamp = t
	})
}

// --- EventOption implementations ---

type eventOptionFunc func(*EventConfig)

func (f eventOptionFunc) applyEvent(c *EventConfig) { f(c) }

// WithEventTimestamp sets the timestamp for an event.
func WithEventTimestamp(t time.Time) EventOption {
	return eventOptionFunc(func(c *EventConfig) {
		c.timestamp = t
	})
}

// WithEventAttributes sets attributes for an event.
func WithEventAttributes(attrs ...Attribute) EventOption {
	return eventOptionFunc(func(c *EventConfig) {
		c.attributes = append(c.attributes, attrs...)
	})
}

// WithStackTrace enables or disables stack trace recording for an event.
func WithStackTrace(record bool) EventOption {
	return eventOptionFunc(func(c *EventConfig) {
		c.stackTrace = record
	})
}

// --- Config accessors for adapters ---

// ApplyRecorderOptions applies options and returns the config.
func ApplyRecorderOptions(opts ...RecorderOption) *RecorderConfig {
	c := &RecorderConfig{}
	for _, opt := range opts {
		opt.applyRecorder(c)
	}
	return c
}

// ApplySpanStartOptions applies options and returns the config.
func ApplySpanStartOptions(opts ...SpanStartOption) *SpanStartConfig {
	c := &SpanStartConfig{}
	for _, opt := range opts {
		opt.applySpanStart(c)
	}
	return c
}

// ApplySpanEndOptions applies options and returns the config.
func ApplySpanEndOptions(opts ...SpanEndOption) *SpanEndConfig {
	c := &SpanEndConfig{}
	for _, opt := range opts {
		opt.applySpanEnd(c)
	}
	return c
}

// ApplyEventOptions applies options and returns the config.
func ApplyEventOptions(opts ...EventOption) *EventConfig {
	c := &EventConfig{}
	for _, opt := range opts {
		opt.applyEvent(c)
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

// LinksIter returns an iterator over the span links.
func (c *SpanStartConfig) LinksIter() iter.Seq[Link] {
	return coreslices.Values(c.links)
}

// AttributesIter returns an iterator over the attributes.
func (c *SpanStartConfig) AttributesIter() iter.Seq[Attribute] {
	return coreslices.Values(c.attributes)
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
	return coreslices.Values(c.attributes)
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
