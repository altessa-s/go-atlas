// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// Adapter implements adapters.Adapter for OTLP export.
type Adapter struct {
	client   otlptrace.Client
	mu       sync.RWMutex
	resource *resourcepb.Resource
	stopped  bool
}

// New creates a new OTLP adapter with the given options.
func New(ctx context.Context, opts ...Option) (*Adapter, error) {
	cfg := defaultOptions()
	for _, opt := range opts {
		opt(cfg)
	}

	// Create client based on protocol
	var client otlptrace.Client

	switch cfg.protocol {
	case ProtocolHTTP:
		client = createHTTPClient(cfg)
	default:
		client = createGRPCClient(cfg)
	}

	// Start the client
	if err := client.Start(ctx); err != nil {
		return nil, coreerrs.WrapOperation(err, "start OTLP client")
	}

	// Create resource
	res := createResource(cfg)

	return &Adapter{
		client:   client,
		resource: res,
	}, nil
}

// createGRPCClient creates an OTLP gRPC client using the custom transport/grpc/client.
func createGRPCClient(cfg *options) otlptrace.Client {
	return newGRPCClient(&grpcClientConfig{
		endpoint:           cfg.endpoint,
		insecure:           cfg.insecure,
		headers:            cfg.headers,
		compression:        cfg.compression,
		exportTimeout:      cfg.exportTimeout,
		retry:              cfg.retry,
		retryConfig:        cfg.retryConfig,
		extraClientOptions: cfg.grpcClientOptions,
	})
}

// createHTTPClient creates an OTLP HTTP client.
func createHTTPClient(cfg *options) otlptrace.Client {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.endpoint),
	}

	opts = slices.AppendIf(opts, cfg.insecure, otlptracehttp.WithInsecure())
	opts = slices.AppendIf(opts, len(cfg.headers) > 0, otlptracehttp.WithHeaders(cfg.headers))
	opts = slices.AppendIf(opts, cfg.compression, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))

	return otlptracehttp.NewClient(opts...)
}

// createResource creates the resource protobuf.
func createResource(cfg *options) *resourcepb.Resource {
	attrs := []*commonpb.KeyValue{
		{
			Key:   "service.name",
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: cfg.serviceName}},
		},
	}

	attrs = slices.AppendIfFunc(attrs, cfg.serviceVersion != "", func() []*commonpb.KeyValue {
		return []*commonpb.KeyValue{{
			Key:   "service.version",
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: cfg.serviceVersion}},
		}}
	})

	attrs = slices.AppendIfFunc(attrs, cfg.environment != "", func() []*commonpb.KeyValue {
		return []*commonpb.KeyValue{{
			Key:   "deployment.environment",
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: cfg.environment}},
		}}
	})

	// Add custom attributes
	for _, attr := range cfg.resourceAttrs {
		attrs = append(attrs, convertAttributeToPB(attr))
	}

	return &resourcepb.Resource{
		Attributes: attrs,
	}
}

// Name implements adapters.Adapter.
func (a *Adapter) Name() string {
	return "otlp"
}

// ExportSpans implements adapters.Adapter.
func (a *Adapter) ExportSpans(ctx context.Context, spans []adapters.SpanData) error {
	a.mu.RLock()
	if a.stopped {
		a.mu.RUnlock()
		return fmt.Errorf("adapter is stopped")
	}
	a.mu.RUnlock()

	if len(spans) == 0 {
		return nil
	}

	// Convert spans to OTLP protobuf format
	resourceSpans := a.convertToResourceSpans(spans)

	// Upload traces
	return a.client.UploadTraces(ctx, resourceSpans)
}

// convertToResourceSpans converts spans to OTLP protobuf format.
func (a *Adapter) convertToResourceSpans(spans []adapters.SpanData) []*tracepb.ResourceSpans {
	// Group spans by instrumentation scope
	scopeSpans := make(map[string]*tracepb.ScopeSpans)

	for i := range spans {
		span := &spans[i]
		scopeKey := ""
		if span.InstrumentationScope != nil {
			scopeKey = span.InstrumentationScope.Name + "@" + span.InstrumentationScope.Version
		}

		ss, ok := scopeSpans[scopeKey]
		if !ok {
			ss = &tracepb.ScopeSpans{}
			if span.InstrumentationScope != nil {
				ss.Scope = &commonpb.InstrumentationScope{
					Name:    span.InstrumentationScope.Name,
					Version: span.InstrumentationScope.Version,
				}
			}
			scopeSpans[scopeKey] = ss
		}

		ss.Spans = append(ss.Spans, convertSpanToPB(span))
	}

	// Build ScopeSpans slice
	scopeSpansList := make([]*tracepb.ScopeSpans, 0, len(scopeSpans))
	for _, ss := range scopeSpans {
		scopeSpansList = append(scopeSpansList, ss)
	}

	return []*tracepb.ResourceSpans{
		{
			Resource:   a.resource,
			ScopeSpans: scopeSpansList,
		},
	}
}

// convertSpanToPB converts a SpanData to OTLP Span protobuf.
func convertSpanToPB(span *adapters.SpanData) *tracepb.Span {
	pbSpan := &tracepb.Span{
		TraceId:                span.TraceID[:],
		SpanId:                 span.SpanID[:],
		Name:                   span.Name,
		Kind:                   convertSpanKindToPB(span.Kind),
		StartTimeUnixNano:      uint64(span.StartTime.UnixNano()),
		EndTimeUnixNano:        uint64(span.EndTime.UnixNano()),
		DroppedAttributesCount: uint32(span.DroppedAttributeCount),
		DroppedEventsCount:     uint32(span.DroppedEventCount),
		DroppedLinksCount:      uint32(span.DroppedLinkCount),
	}

	// Set parent span ID if present
	if span.ParentID.IsValid() {
		pbSpan.ParentSpanId = span.ParentID[:]
	}

	// Convert attributes
	if len(span.Attributes) > 0 {
		pbSpan.Attributes = make([]*commonpb.KeyValue, len(span.Attributes))
		for i, attr := range span.Attributes {
			pbSpan.Attributes[i] = convertAttributeToPB(attr)
		}
	}

	// Convert events
	if len(span.Events) > 0 {
		pbSpan.Events = make([]*tracepb.Span_Event, len(span.Events))
		for i, event := range span.Events {
			pbSpan.Events[i] = convertEventToPB(event)
		}
	}

	// Convert links
	if len(span.Links) > 0 {
		pbSpan.Links = make([]*tracepb.Span_Link, len(span.Links))
		for i, link := range span.Links {
			pbSpan.Links[i] = convertLinkToPB(link)
		}
	}

	// Convert status
	pbSpan.Status = &tracepb.Status{
		Code:    convertStatusToPB(span.Status),
		Message: span.StatusDesc,
	}

	return pbSpan
}

// convertSpanKindToPB converts SpanKind to OTLP SpanKind.
func convertSpanKindToPB(kind adapters.SpanKind) tracepb.Span_SpanKind {
	switch kind {
	case adapters.SpanKindInternal:
		return tracepb.Span_SPAN_KIND_INTERNAL
	case adapters.SpanKindServer:
		return tracepb.Span_SPAN_KIND_SERVER
	case adapters.SpanKindClient:
		return tracepb.Span_SPAN_KIND_CLIENT
	case adapters.SpanKindProducer:
		return tracepb.Span_SPAN_KIND_PRODUCER
	case adapters.SpanKindConsumer:
		return tracepb.Span_SPAN_KIND_CONSUMER
	default:
		return tracepb.Span_SPAN_KIND_UNSPECIFIED
	}
}

// convertStatusToPB converts StatusCode to OTLP StatusCode.
func convertStatusToPB(status adapters.StatusCode) tracepb.Status_StatusCode {
	switch status {
	case adapters.StatusOK:
		return tracepb.Status_STATUS_CODE_OK
	case adapters.StatusError:
		return tracepb.Status_STATUS_CODE_ERROR
	default:
		return tracepb.Status_STATUS_CODE_UNSET
	}
}

// convertEventToPB converts SpanEvent to OTLP Event.
func convertEventToPB(event adapters.SpanEvent) *tracepb.Span_Event {
	pbEvent := &tracepb.Span_Event{
		Name:         event.Name,
		TimeUnixNano: uint64(event.Timestamp.UnixNano()),
	}

	if len(event.Attributes) > 0 {
		pbEvent.Attributes = make([]*commonpb.KeyValue, len(event.Attributes))
		for i, attr := range event.Attributes {
			pbEvent.Attributes[i] = convertAttributeToPB(attr)
		}
	}

	return pbEvent
}

// convertLinkToPB converts SpanLink to OTLP Link.
func convertLinkToPB(link adapters.SpanLink) *tracepb.Span_Link {
	pbLink := &tracepb.Span_Link{
		TraceId:    link.TraceID[:],
		SpanId:     link.SpanID[:],
		TraceState: link.TraceState,
	}

	if len(link.Attributes) > 0 {
		pbLink.Attributes = make([]*commonpb.KeyValue, len(link.Attributes))
		for i, attr := range link.Attributes {
			pbLink.Attributes[i] = convertAttributeToPB(attr)
		}
	}

	return pbLink
}

// convertAttributeToPB converts Attribute to OTLP KeyValue.
func convertAttributeToPB(attr adapters.Attribute) *commonpb.KeyValue {
	kv := &commonpb.KeyValue{
		Key: attr.Key,
	}

	switch v := attr.Value.(type) {
	case string:
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}
	case int:
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: int64(v)}}
	case int64:
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: v}}
	case float64:
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: v}}
	case bool:
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: v}}
	case []string:
		values := make([]*commonpb.AnyValue, len(v))
		for i, s := range v {
			values[i] = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
		}
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
			ArrayValue: &commonpb.ArrayValue{Values: values},
		}}
	case []int:
		values := make([]*commonpb.AnyValue, len(v))
		for i, n := range v {
			values[i] = &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: int64(n)}}
		}
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
			ArrayValue: &commonpb.ArrayValue{Values: values},
		}}
	case []int64:
		values := make([]*commonpb.AnyValue, len(v))
		for i, n := range v {
			values[i] = &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: n}}
		}
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
			ArrayValue: &commonpb.ArrayValue{Values: values},
		}}
	case []float64:
		values := make([]*commonpb.AnyValue, len(v))
		for i, f := range v {
			values[i] = &commonpb.AnyValue{Value: &commonpb.AnyValue_DoubleValue{DoubleValue: f}}
		}
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
			ArrayValue: &commonpb.ArrayValue{Values: values},
		}}
	case []bool:
		values := make([]*commonpb.AnyValue, len(v))
		for i, b := range v {
			values[i] = &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: b}}
		}
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_ArrayValue{
			ArrayValue: &commonpb.ArrayValue{Values: values},
		}}
	default:
		kv.Value = &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: fmt.Sprintf("%v", v)}}
	}

	return kv
}

// Shutdown implements adapters.Adapter.
func (a *Adapter) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return nil
	}
	a.stopped = true

	return a.client.Stop(ctx)
}

// ForceFlush implements adapters.Adapter.
func (a *Adapter) ForceFlush(ctx context.Context) error {
	a.mu.RLock()
	if a.stopped {
		a.mu.RUnlock()
		return nil
	}
	a.mu.RUnlock()

	// The OTLP client doesn't have a flush method,
	// but UploadTraces is synchronous
	return nil
}

// Ensure Adapter implements the interface.
var _ adapters.Adapter = (*Adapter)(nil)
