// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"context"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
)

// recordingSpan is a Span that records events and attributes.
type recordingSpan struct {
	mu          sync.RWMutex
	recorder    *recorder
	name        string
	spanContext *spanContextImpl
	parentID    [8]byte
	kind        SpanKind
	startTime   time.Time
	endTime     time.Time
	attributes  []Attribute
	events      []adapters.SpanEvent
	links       []Link
	status      StatusCode
	statusDesc  string
	ended       bool

	droppedAttrCount  int
	droppedEventCount int
	droppedLinkCount  int
}

// End implements Span.
func (s *recordingSpan) End(opts ...SpanEndOption) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended {
		return
	}
	s.ended = true

	cfg := ApplySpanEndOptions(opts...)
	if cfg.Timestamp().IsZero() {
		s.endTime = time.Now()
	} else {
		s.endTime = cfg.Timestamp()
	}

	// Export the span
	s.export()
}

// SpanContext implements Span.
func (s *recordingSpan) SpanContext() SpanContext {
	return s.spanContext
}

// IsRecording implements Span.
func (s *recordingSpan) IsRecording() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return !s.ended
}

// SetName implements Span.
func (s *recordingSpan) SetName(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		s.name = name
	}
}

// SetStatus implements Span.
func (s *recordingSpan) SetStatus(code StatusCode, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		// Only update if setting a higher priority status
		// Error > OK > Unset
		if code > s.status {
			s.status = code
			s.statusDesc = description
		}
	}
}

// SetAttributes implements Span.
func (s *recordingSpan) SetAttributes(attrs ...Attribute) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		s.attributes = append(s.attributes, attrs...)
	}
}

// RecordError implements Span.
func (s *recordingSpan) RecordError(err error, opts ...EventOption) {
	if err == nil {
		return
	}

	cfg := ApplyEventOptions(opts...)
	timestamp := cfg.Timestamp()
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	// Build error attributes
	errorAttrs := []adapters.Attribute{
		{Key: "exception.type", Value: errorType(err)},
		{Key: "exception.message", Value: err.Error()},
	}
	for _, attr := range cfg.Attributes() {
		errorAttrs = append(errorAttrs, adapters.Attribute{
			Key:   attr.Key,
			Value: attr.Value,
		})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.ended {
		s.events = append(s.events, adapters.SpanEvent{
			Name:       "exception",
			Timestamp:  timestamp,
			Attributes: errorAttrs,
		})
	}
}

// AddEvent implements Span.
func (s *recordingSpan) AddEvent(name string, opts ...EventOption) {
	cfg := ApplyEventOptions(opts...)
	timestamp := cfg.Timestamp()
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	eventAttrs := make([]adapters.Attribute, 0, len(cfg.Attributes()))
	for _, attr := range cfg.Attributes() {
		eventAttrs = append(eventAttrs, adapters.Attribute{
			Key:   attr.Key,
			Value: attr.Value,
		})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.ended {
		s.events = append(s.events, adapters.SpanEvent{
			Name:       name,
			Timestamp:  timestamp,
			Attributes: eventAttrs,
		})
	}
}

// export sends the span data to adapters.
func (s *recordingSpan) export() {
	// Convert attributes to adapter format
	adapterAttrs := make([]adapters.Attribute, len(s.attributes))
	for i, attr := range s.attributes {
		adapterAttrs[i] = adapters.Attribute{Key: attr.Key, Value: attr.Value}
	}

	// Convert links to adapter format
	adapterLinks := make([]adapters.SpanLink, len(s.links))
	for i, link := range s.links {
		linkAttrs := make([]adapters.Attribute, len(link.Attributes))
		for j, attr := range link.Attributes {
			linkAttrs[j] = adapters.Attribute{Key: attr.Key, Value: attr.Value}
		}
		adapterLinks[i] = adapters.SpanLink{
			TraceID:    parseTraceID(link.SpanContext.TraceID()),
			SpanID:     parseSpanID(link.SpanContext.SpanID()),
			Attributes: linkAttrs,
		}
	}

	spanData := &adapters.SpanData{
		TraceID:    s.spanContext.traceID,
		SpanID:     s.spanContext.spanID,
		ParentID:   s.parentID,
		Name:       s.name,
		Kind:       adapters.SpanKind(s.kind),
		StartTime:  s.startTime,
		EndTime:    s.endTime,
		Attributes: adapterAttrs,
		Events:     s.events,
		Links:      adapterLinks,
		Status:     adapters.StatusCode(s.status),
		StatusDesc: s.statusDesc,
		Resource: &adapters.Resource{
			Attributes: []adapters.Attribute{
				{Key: "service.name", Value: s.recorder.tracer.serviceName},
				{Key: "service.version", Value: s.recorder.tracer.serviceVersion},
				{Key: "deployment.environment", Value: s.recorder.tracer.environment},
			},
		},
		InstrumentationScope: &adapters.InstrumentationScope{
			Name:      s.recorder.name,
			Version:   s.recorder.config.InstrumentationVersion(),
			SchemaURL: s.recorder.config.SchemaURL(),
		},
		DroppedAttributeCount: s.droppedAttrCount,
		DroppedEventCount:     s.droppedEventCount,
		DroppedLinkCount:      s.droppedLinkCount,
	}

	// Export to tracer
	s.recorder.tracer.export(context.Background(), spanData)
}

// errorType extracts the type name from an error.
func errorType(err error) string {
	if err == nil {
		return ""
	}
	// Use the full type path
	return "*errors.errorString"
}

// Ensure recordingSpan implements Span.
var _ Span = (*recordingSpan)(nil)
