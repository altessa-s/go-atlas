// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package adapters

import (
	"context"
	"iter"
	"time"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// TraceID is a 16-byte trace identifier.
type TraceID [16]byte

// SpanID is an 8-byte span identifier.
type SpanID [8]byte

// IsValid returns true if the TraceID is not zero.
func (t TraceID) IsValid() bool {
	return t != TraceID{}
}

// IsValid returns true if the SpanID is not zero.
func (s SpanID) IsValid() bool {
	return s != SpanID{}
}

// String returns the hex representation of TraceID.
func (t TraceID) String() string {
	return encodeHex(t[:])
}

// String returns the hex representation of SpanID.
func (s SpanID) String() string {
	return encodeHex(s[:])
}

// encodeHex encodes bytes to hex string.
func encodeHex(b []byte) string {
	const hexChars = "0123456789abcdef"
	buf := make([]byte, len(b)*2)
	for i, v := range b {
		buf[i*2] = hexChars[v>>4]
		buf[i*2+1] = hexChars[v&0x0f]
	}
	return string(buf)
}

// SpanKind represents the role of the span in a trace.
type SpanKind int

const (
	SpanKindUnspecified SpanKind = iota
	SpanKindInternal
	SpanKindServer
	SpanKindClient
	SpanKindProducer
	SpanKindConsumer
)

// String returns the string representation of the span kind.
func (k SpanKind) String() string {
	switch k {
	case SpanKindInternal:
		return "internal"
	case SpanKindServer:
		return "server"
	case SpanKindClient:
		return "client"
	case SpanKindProducer:
		return "producer"
	case SpanKindConsumer:
		return "consumer"
	default:
		return "unspecified"
	}
}

// StatusCode represents the status of a span.
type StatusCode int

const (
	StatusUnset StatusCode = iota
	StatusOK
	StatusError
)

// String returns the string representation of the status code.
func (c StatusCode) String() string {
	switch c {
	case StatusOK:
		return "OK"
	case StatusError:
		return "Error"
	default:
		return "Unset"
	}
}

// Attribute represents a key-value pair.
type Attribute struct {
	Key   string
	Value any
}

// SpanEvent represents an event that occurred during a span.
type SpanEvent struct {
	Name       string
	Timestamp  time.Time
	Attributes []Attribute
}

// SpanLink represents a link to another span.
type SpanLink struct {
	TraceID    TraceID
	SpanID     SpanID
	TraceState string
	Attributes []Attribute
}

// Resource represents the entity producing telemetry.
type Resource struct {
	Attributes []Attribute
	SchemaURL  string
}

// InstrumentationScope represents the instrumentation scope.
type InstrumentationScope struct {
	Name      string
	Version   string
	SchemaURL string
}

// SpanData contains all data for an exported span.
// This is the data structure passed to [Adapter.ExportSpans] for export.
type SpanData struct {
	// Identity
	TraceID  TraceID
	SpanID   SpanID
	ParentID SpanID

	// Naming
	Name string
	Kind SpanKind

	// Timing
	StartTime time.Time
	EndTime   time.Time

	// Attributes and events
	Attributes []Attribute
	Events     []SpanEvent
	Links      []SpanLink

	// Status
	Status     StatusCode
	StatusDesc string

	// Context
	Resource             *Resource
	InstrumentationScope *InstrumentationScope

	// Flags
	DroppedAttributeCount int
	DroppedEventCount     int
	DroppedLinkCount      int
}

// Duration returns the span duration.
func (s *SpanData) Duration() time.Duration {
	return s.EndTime.Sub(s.StartTime)
}

// AttributesIter returns an iterator over span attributes.
func (s *SpanData) AttributesIter() iter.Seq[Attribute] {
	return coreslices.Values(s.Attributes)
}

// EventsIter returns an iterator over span events.
func (s *SpanData) EventsIter() iter.Seq[SpanEvent] {
	return coreslices.Values(s.Events)
}

// LinksIter returns an iterator over span links.
func (s *SpanData) LinksIter() iter.Seq[SpanLink] {
	return coreslices.Values(s.Links)
}

// Adapter is the interface that tracing backends must implement.
// It exports [SpanData] to specific backend formats.
// Implementations must be safe for concurrent use.
type Adapter interface {
	// Name returns the adapter name (e.g., "otlp", "console", "jaeger").
	Name() string

	// ExportSpans exports a batch of spans to the backend.
	// The context may be used for cancellation and deadlines.
	ExportSpans(ctx context.Context, spans []SpanData) error

	// Shutdown shuts down the adapter, flushing any remaining data.
	// After Shutdown is called, ExportSpans should not be called.
	Shutdown(ctx context.Context) error

	// ForceFlush forces an immediate export of any buffered spans.
	ForceFlush(ctx context.Context) error
}

// MultiAdapter wraps multiple [Adapter] instances to broadcast span exports.
// Not safe for concurrent modification after construction; concurrent
// method calls are safe.
type MultiAdapter struct {
	adapters []Adapter
}

// NewMultiAdapter creates an adapter that broadcasts to multiple adapters.
func NewMultiAdapter(adapters ...Adapter) *MultiAdapter {
	return &MultiAdapter{adapters: adapters}
}

// Name implements Adapter.
func (m *MultiAdapter) Name() string {
	return "multi"
}

// ExportSpans implements Adapter.
// Broadcasts span export to all adapters.
func (m *MultiAdapter) ExportSpans(ctx context.Context, spans []SpanData) error {
	for a := range coreslices.Values(m.adapters) {
		if err := a.ExportSpans(ctx, spans); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown implements Adapter.
// Shuts down all adapters, returning the first error encountered.
func (m *MultiAdapter) Shutdown(ctx context.Context) error {
	for a := range coreslices.Values(m.adapters) {
		if err := a.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ForceFlush implements Adapter.
// Flushes all adapters, returning the first error encountered.
func (m *MultiAdapter) ForceFlush(ctx context.Context) error {
	for a := range coreslices.Values(m.adapters) {
		if err := a.ForceFlush(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Ensure MultiAdapter implements Adapter.
var _ Adapter = (*MultiAdapter)(nil)
