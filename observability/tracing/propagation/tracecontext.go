// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package propagation

import (
	"context"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/altessa-s/go-atlas/observability/tracing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

const (
	// W3C Trace Context header names
	traceparentHeader = "traceparent"
	tracestateHeader  = "tracestate"

	// traceparent format version
	supportedVersion = "00"
)

// traceContextRegex matches valid traceparent header values.
// Format: version-traceid-spanid-traceflags
// Example: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
var traceContextRegex = regexp.MustCompile(
	`^([0-9a-f]{2})-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})$`,
)

// TraceContext implements W3C Trace Context propagation.
// See: https://www.w3.org/TR/trace-context/
type TraceContext struct{}

// NewTraceContext creates a new W3C Trace Context propagator.
func NewTraceContext() *TraceContext {
	return &TraceContext{}
}

// Inject implements TextMapPropagator.
func (tc *TraceContext) Inject(ctx context.Context, carrier TextMapCarrier) {
	span := tracing.SpanFromContext(ctx)
	sc := span.SpanContext()

	if sc == nil || !sc.IsValid() {
		return
	}

	// Format: version-traceid-spanid-traceflags
	// Example: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
	flags := "00"
	if sc.IsSampled() {
		flags = "01"
	}

	traceparent := supportedVersion + "-" + sc.TraceID() + "-" + sc.SpanID() + "-" + flags
	carrier.Set(traceparentHeader, traceparent)

	// Inject tracestate if present (from span context implementation)
	// For now, we don't have tracestate in our SpanContext interface
}

// Extract implements TextMapPropagator.
func (tc *TraceContext) Extract(ctx context.Context, carrier TextMapCarrier) context.Context {
	traceparent := carrier.Get(traceparentHeader)
	if traceparent == "" {
		return ctx
	}

	sc, ok := parseTraceParent(traceparent)
	if !ok {
		return ctx
	}

	// Parse tracestate if present
	tracestate := carrier.Get(tracestateHeader)
	if tracestate != "" {
		sc.traceState = tracestate
	}

	// Create a remote span context and store in context
	return contextWithRemoteSpanContext(ctx, sc)
}

// Fields implements TextMapPropagator.
func (tc *TraceContext) Fields() []string {
	return []string{traceparentHeader, tracestateHeader}
}

// spanContextData holds parsed span context data.
type spanContextData struct {
	traceID    string
	spanID     string
	traceFlags byte
	traceState string
	remote     bool
}

// parseTraceParent parses a traceparent header value.
func parseTraceParent(header string) (spanContextData, bool) {
	header = strings.TrimSpace(header)
	header = corestrings.InternLowerString(header)

	matches := traceContextRegex.FindStringSubmatch(header)
	if matches == nil {
		return spanContextData{}, false
	}

	version := matches[1]
	traceID := matches[2]
	spanID := matches[3]
	flagsHex := matches[4]

	// Check version
	if version != supportedVersion {
		// Unknown version, but we can still try to parse
		if version == "ff" {
			return spanContextData{}, false // Invalid version
		}
	}

	// Validate trace ID (must not be all zeros)
	if traceID == "00000000000000000000000000000000" {
		return spanContextData{}, false
	}

	// Validate span ID (must not be all zeros)
	if spanID == "0000000000000000" {
		return spanContextData{}, false
	}

	// Parse flags
	flags, err := hex.DecodeString(flagsHex)
	if err != nil || len(flags) != 1 {
		return spanContextData{}, false
	}

	return spanContextData{
		traceID:    traceID,
		spanID:     spanID,
		traceFlags: flags[0],
		remote:     true,
	}, true
}

// remoteSpanContextKey is the context key for remote span context.
type remoteSpanContextKey struct{}

// contextWithRemoteSpanContext stores remote span context in context.
func contextWithRemoteSpanContext(ctx context.Context, sc spanContextData) context.Context {
	return context.WithValue(ctx, remoteSpanContextKey{}, sc)
}

// RemoteSpanContextFromContext retrieves remote span context from context.
// Returns nil if not present.
func RemoteSpanContextFromContext(ctx context.Context) *RemoteSpanContext {
	if sc, ok := ctx.Value(remoteSpanContextKey{}).(spanContextData); ok {
		return &RemoteSpanContext{data: sc}
	}
	return nil
}

// RemoteSpanContext represents a span context received from an external source.
type RemoteSpanContext struct {
	data spanContextData
}

// TraceID returns the trace ID.
func (r *RemoteSpanContext) TraceID() string {
	return r.data.traceID
}

// SpanID returns the span ID.
func (r *RemoteSpanContext) SpanID() string {
	return r.data.spanID
}

// TraceFlags returns the trace flags.
func (r *RemoteSpanContext) TraceFlags() tracing.TraceFlags {
	return tracing.TraceFlags(r.data.traceFlags)
}

// IsSampled returns true if the sampled flag is set.
func (r *RemoteSpanContext) IsSampled() bool {
	return r.data.traceFlags&byte(tracing.FlagsSampled) == byte(tracing.FlagsSampled)
}

// IsValid returns true if the span context is valid.
func (r *RemoteSpanContext) IsValid() bool {
	return r.data.traceID != "" && r.data.spanID != ""
}

// IsRemote returns true (always remote).
func (r *RemoteSpanContext) IsRemote() bool {
	return true
}

// TraceState returns the trace state.
func (r *RemoteSpanContext) TraceState() string {
	return r.data.traceState
}

// Ensure TraceContext implements TextMapPropagator.
var _ TextMapPropagator = (*TraceContext)(nil)
