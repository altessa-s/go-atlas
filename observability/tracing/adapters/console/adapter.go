// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package console

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
)

// Adapter implements adapters.Adapter for console output.
// It outputs spans to a writer in a human-readable or JSON format.
type Adapter struct {
	mu          sync.Mutex
	writer      io.Writer
	prettyPrint bool
	timestamps  bool
}

// New creates a new console adapter with the given options.
func New(opts ...Option) *Adapter {
	cfg := defaultOptions()
	for _, opt := range opts {
		opt(cfg)
	}

	writer := cfg.writer
	if writer == nil {
		writer = os.Stdout
	}

	return &Adapter{
		writer:      writer,
		prettyPrint: cfg.prettyPrint,
		timestamps:  cfg.timestamps,
	}
}

// Name implements adapters.Adapter.
func (a *Adapter) Name() string {
	return "console"
}

// ExportSpans implements adapters.Adapter.
func (a *Adapter) ExportSpans(ctx context.Context, spans []adapters.SpanData) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, span := range spans {
		if err := a.writeSpan(&span); err != nil {
			return err
		}
	}

	return nil
}

// writeSpan writes a single span to the output.
func (a *Adapter) writeSpan(span *adapters.SpanData) error {
	if a.prettyPrint {
		return a.writePretty(span)
	}
	return a.writeJSON(span)
}

// writePretty writes a span in a human-readable format.
func (a *Adapter) writePretty(span *adapters.SpanData) error {
	var prefix string
	if a.timestamps {
		prefix = time.Now().Format("15:04:05.000") + " "
	}

	// Format: [SPAN] name (trace_id/span_id) duration status
	_, err := fmt.Fprintf(a.writer,
		"%s[SPAN] %s (%s/%s) %s %s\n",
		prefix,
		span.Name,
		span.TraceID.String()[:16], // First 16 chars of trace ID
		span.SpanID.String(),
		span.Duration().Round(time.Microsecond),
		span.Status.String(),
	)
	if err != nil {
		return err
	}

	// Print attributes
	for attr := range span.AttributesIter() {
		if _, err := fmt.Fprintf(a.writer, "%s       %s=%v\n", prefix, attr.Key, attr.Value); err != nil {
			return err
		}
	}

	// Print events
	for event := range span.EventsIter() {
		if _, err := fmt.Fprintf(a.writer, "%s  [EVENT] %s at %s\n",
			prefix, event.Name, event.Timestamp.Format("15:04:05.000")); err != nil {
			return err
		}
	}

	return nil
}

// spanJSON is the JSON representation of a span for console output.
type spanJSON struct {
	TraceID    string         `json:"trace_id"`
	SpanID     string         `json:"span_id"`
	ParentID   string         `json:"parent_id,omitempty"`
	Name       string         `json:"name"`
	Kind       string         `json:"kind"`
	StartTime  string         `json:"start_time"`
	EndTime    string         `json:"end_time"`
	Duration   string         `json:"duration"`
	Status     string         `json:"status"`
	StatusDesc string         `json:"status_description,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
	Events     []eventJSON    `json:"events,omitempty"`
	Links      []linkJSON     `json:"links,omitempty"`
}

type eventJSON struct {
	Name       string         `json:"name"`
	Timestamp  string         `json:"timestamp"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type linkJSON struct {
	TraceID    string         `json:"trace_id"`
	SpanID     string         `json:"span_id"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// writeJSON writes a span in JSON format.
func (a *Adapter) writeJSON(span *adapters.SpanData) error {
	sj := spanJSON{
		TraceID:    span.TraceID.String(),
		SpanID:     span.SpanID.String(),
		Name:       span.Name,
		Kind:       span.Kind.String(),
		StartTime:  span.StartTime.Format(time.RFC3339Nano),
		EndTime:    span.EndTime.Format(time.RFC3339Nano),
		Duration:   span.Duration().String(),
		Status:     span.Status.String(),
		StatusDesc: span.StatusDesc,
	}

	if span.ParentID.IsValid() {
		sj.ParentID = span.ParentID.String()
	}

	// Convert attributes
	if len(span.Attributes) > 0 {
		sj.Attributes = make(map[string]any, len(span.Attributes))
		for _, attr := range span.Attributes {
			sj.Attributes[attr.Key] = attr.Value
		}
	}

	// Convert events
	if len(span.Events) > 0 {
		sj.Events = make([]eventJSON, len(span.Events))
		for i, event := range span.Events {
			sj.Events[i] = eventJSON{
				Name:      event.Name,
				Timestamp: event.Timestamp.Format(time.RFC3339Nano),
			}
			if len(event.Attributes) > 0 {
				sj.Events[i].Attributes = make(map[string]any, len(event.Attributes))
				for _, attr := range event.Attributes {
					sj.Events[i].Attributes[attr.Key] = attr.Value
				}
			}
		}
	}

	// Convert links
	if len(span.Links) > 0 {
		sj.Links = make([]linkJSON, len(span.Links))
		for i, link := range span.Links {
			sj.Links[i] = linkJSON{
				TraceID: link.TraceID.String(),
				SpanID:  link.SpanID.String(),
			}
			if len(link.Attributes) > 0 {
				sj.Links[i].Attributes = make(map[string]any, len(link.Attributes))
				for _, attr := range link.Attributes {
					sj.Links[i].Attributes[attr.Key] = attr.Value
				}
			}
		}
	}

	data, err := json.Marshal(sj)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(a.writer, string(data))
	return err
}

// Shutdown implements adapters.Adapter.
func (a *Adapter) Shutdown(_ context.Context) error {
	return nil
}

// ForceFlush implements adapters.Adapter.
func (a *Adapter) ForceFlush(_ context.Context) error {
	return nil
}

// Ensure Adapter implements the interface.
var _ adapters.Adapter = (*Adapter)(nil)
