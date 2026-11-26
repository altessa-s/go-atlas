// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/sampler"
)

const stackTraceAttrKey = "exception.stacktrace"

// recordingSpan is a Span that records events and attributes.
type recordingSpan struct {
	mu                sync.RWMutex
	recorder          *recorder
	name              string
	spanContext       *spanContextImpl
	parentID          [8]byte
	kind              SpanKind
	startTime         time.Time
	endTime           time.Time
	attributes        []Attribute
	events            []adapters.SpanEvent
	links             []Link
	droppedAttributes int
	droppedEvents     int
	droppedLinks      int
	status            StatusCode
	statusDesc        string
	ended             bool
}

// End implements Span.
func (s *recordingSpan) End(opts ...SpanEndOption) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ended {
		return
	}
	s.ended = true

	if len(opts) > 0 {
		s.endTime = ApplySpanEndOptions(opts...).Timestamp()
	}
	if s.endTime.IsZero() {
		s.endTime = time.Now()
	}

	// RecordOnly spans remain available locally but must not be exported.
	if s.spanContext.IsSampled() {
		s.export()
	}
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
	if len(attrs) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ended {
		s.putAttributes(attrs...)
	}
}

func (s *recordingSpan) putAttributes(attrs ...Attribute) {
	maxAttrs := s.recorder.tracer.limits.maxAttributes
	for _, attr := range attrs {
		s.putOneAttribute(attr, maxAttrs)
	}
}

func (s *recordingSpan) putSamplerAttributes(attrs []sampler.Attribute) {
	maxAttrs := s.recorder.tracer.limits.maxAttributes
	for _, attr := range attrs {
		s.putOneAttribute(Attribute{Key: attr.Key, Value: attr.Value}, maxAttrs)
	}
}

func (s *recordingSpan) putOneAttribute(attr Attribute, maxAttrs int) {
	for i := range s.attributes {
		if s.attributes[i].Key == attr.Key {
			s.attributes[i].Value = attr.Value
			return
		}
	}
	if maxAttrs > 0 && len(s.attributes) >= maxAttrs {
		s.droppedAttributes++
		return
	}
	s.attributes = append(s.attributes, attr)
}

// RecordError implements Span.
func (s *recordingSpan) RecordError(err error, opts ...EventOption) {
	if err == nil || !s.IsRecording() {
		return
	}

	cfg := ApplyEventOptions(opts...)
	timestamp := cfg.Timestamp()
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	// Build error attributes
	errorAttrs := []adapters.Attribute{
		{Key: "exception.type", Value: fmt.Sprintf("%T", err)},
		{Key: "exception.message", Value: err.Error()},
	}
	var hasStackAttr bool
	for _, attr := range cfg.Attributes() {
		if attr.Key == "exception.message" {
			errorAttrs[1].Value = attr.Value
			continue
		}
		if attr.Key == stackTraceAttrKey {
			hasStackAttr = true
		}
		errorAttrs = append(errorAttrs, adapters.Attribute{
			Key:   attr.Key,
			Value: attr.Value,
		})
	}
	if cfg.StackTrace() && !hasStackAttr {
		errorAttrs = append(errorAttrs, adapters.Attribute{
			Key:   stackTraceAttrKey,
			Value: captureStackTrace(),
		})
	}

	s.recordEvent(adapters.SpanEvent{
		Name:       "exception",
		Timestamp:  timestamp,
		Attributes: errorAttrs,
	})
}

// AddEvent implements Span.
func (s *recordingSpan) AddEvent(name string, opts ...EventOption) {
	if !s.IsRecording() {
		return
	}

	cfg := ApplyEventOptions(opts...)
	timestamp := cfg.Timestamp()
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	eventAttrs := make([]adapters.Attribute, 0, len(cfg.Attributes()))
	var hasStackAttr bool
	for _, attr := range cfg.Attributes() {
		if attr.Key == stackTraceAttrKey {
			hasStackAttr = true
		}
		eventAttrs = append(eventAttrs, adapters.Attribute{
			Key:   attr.Key,
			Value: attr.Value,
		})
	}
	if cfg.StackTrace() && !hasStackAttr {
		eventAttrs = append(eventAttrs, adapters.Attribute{
			Key:   stackTraceAttrKey,
			Value: captureStackTrace(),
		})
	}

	s.recordEvent(adapters.SpanEvent{
		Name:       name,
		Timestamp:  timestamp,
		Attributes: eventAttrs,
	})
}

func (s *recordingSpan) recordEvent(ev adapters.SpanEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	if maxEv := s.recorder.tracer.limits.maxEvents; maxEv > 0 && len(s.events) >= maxEv {
		s.droppedEvents++
		return
	}
	s.events = append(s.events, ev)
}

func captureStackTrace() string {
	var pcs [64]uintptr
	// Skip runtime.Callers, captureStackTrace, and RecordError/AddEvent.
	n := runtime.Callers(3, pcs[:]) //nolint:mnd // Skip internal stack-capture frames
	if n == 0 {
		return ""
	}
	frames := runtime.CallersFrames(pcs[:n])
	var b strings.Builder
	for {
		frame, more := frames.Next()
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(frame.Function)
		b.WriteString("\n\t")
		b.WriteString(frame.File)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(frame.Line))
		if !more {
			break
		}
	}
	return b.String()
}

// export snapshots the ended span for the batch worker.
func (s *recordingSpan) export() {
	// Convert attributes to adapter format
	adapterAttrs := make([]adapters.Attribute, len(s.attributes))
	for i, attr := range s.attributes {
		adapterAttrs[i] = adapters.Attribute{Key: attr.Key, Value: snapshotValue(attr.Value)}
	}

	// Convert links to adapter format
	adapterLinks := make([]adapters.SpanLink, 0, len(s.links))
	for _, link := range s.links {
		if !isValidSpanContext(link.SpanContext) {
			continue
		}
		linkAttrs := make([]adapters.Attribute, len(link.Attributes))
		for j, attr := range link.Attributes {
			linkAttrs[j] = adapters.Attribute{Key: attr.Key, Value: snapshotValue(attr.Value)}
		}
		adapterLinks = append(adapterLinks, adapters.SpanLink{
			TraceID:    traceIDOf(link.SpanContext),
			SpanID:     spanIDOf(link.SpanContext),
			TraceState: traceStateOf(link.SpanContext),
			Attributes: linkAttrs,
		})
	}

	for i := range s.events {
		for j := range s.events[i].Attributes {
			attr := &s.events[i].Attributes[j]
			attr.Value = snapshotValue(attr.Value)
		}
	}

	spanData := &adapters.SpanData{
		TraceID:               s.spanContext.traceID,
		SpanID:                s.spanContext.spanID,
		ParentID:              s.parentID,
		Name:                  s.name,
		Kind:                  adapters.SpanKind(s.kind),
		StartTime:             s.startTime,
		EndTime:               s.endTime,
		Attributes:            adapterAttrs,
		Events:                s.events,
		Links:                 adapterLinks,
		Status:                adapters.StatusCode(s.status),
		StatusDesc:            s.statusDesc,
		Resource:              s.recorder.tracer.resource,
		InstrumentationScope:  s.recorder.scope,
		DroppedAttributeCount: s.droppedAttributes,
		DroppedEventCount:     s.droppedEvents,
		DroppedLinkCount:      s.droppedLinks,
	}

	s.recorder.tracer.processor.enqueue(spanData)
}

// Ensure recordingSpan implements Span.
var _ Span = (*recordingSpan)(nil)
