// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

// requestCollector records every Export request and optionally fails them.
type requestCollector struct {
	coltracepb.UnimplementedTraceServiceServer
	got  chan *coltracepb.ExportTraceServiceRequest
	fail error
}

func (c *requestCollector) Export(_ context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	if c.fail != nil {
		return nil, c.fail
	}
	c.got <- proto.Clone(req).(*coltracepb.ExportTraceServiceRequest)
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func startRequestCollector(t *testing.T, fail error) (*requestCollector, *grpc.ClientConn) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	col := &requestCollector{got: make(chan *coltracepb.ExportTraceServiceRequest, 8), fail: fail}
	srv := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(srv, col)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return col, conn
}

func attrMap(kvs []*commonpb.KeyValue) map[string]*commonpb.AnyValue {
	m := make(map[string]*commonpb.AnyValue, len(kvs))
	for _, kv := range kvs {
		m[kv.GetKey()] = kv.GetValue()
	}
	return m
}

type stringer struct{}

func (stringer) String() string { return "custom" }

func TestExportSpans_ConvertsFullSpan(t *testing.T) {
	t.Parallel()
	col, conn := startRequestCollector(t, nil)

	a, err := otlp.New(t.Context(),
		otlp.WithGRPCConn(conn),
		otlp.WithCompression(false),
		otlp.WithExportTimeout(5*time.Second),
		otlp.WithServiceName("svc"),
		otlp.WithServiceVersion("1.2.3"),
		otlp.WithEnvironment("prod"),
		otlp.WithResourceAttrs(adapters.Attribute{Key: "region", Value: "eu"}),
	)
	require.NoError(t, err)
	require.Equal(t, "otlp", a.Name())

	start := time.Unix(100, 0)
	end := start.Add(time.Second)
	span := adapters.SpanData{
		TraceID:   adapters.TraceID{0x01},
		SpanID:    adapters.SpanID{0x02},
		ParentID:  adapters.SpanID{0x03},
		Name:      "op",
		Kind:      adapters.SpanKindServer,
		StartTime: start,
		EndTime:   end,
		Attributes: []adapters.Attribute{
			{Key: "s", Value: "str"},
			{Key: "i", Value: 7},
			{Key: "i64", Value: int64(8)},
			{Key: "f", Value: 1.5},
			{Key: "b", Value: true},
			{Key: "ss", Value: []string{"a", "b"}},
			{Key: "is", Value: []int{1, 2}},
			{Key: "i64s", Value: []int64{3}},
			{Key: "fs", Value: []float64{0.5}},
			{Key: "bs", Value: []bool{true, false}},
			{Key: "other", Value: stringer{}},
		},
		Events: []adapters.SpanEvent{
			{Name: "ev", Timestamp: start, Attributes: []adapters.Attribute{{Key: "ek", Value: "ev"}}},
			{Name: "bare", Timestamp: end},
		},
		Links: []adapters.SpanLink{
			{TraceID: adapters.TraceID{0x09}, SpanID: adapters.SpanID{0x08}, TraceState: "k=v",
				Attributes: []adapters.Attribute{{Key: "lk", Value: false}}},
			{TraceID: adapters.TraceID{0x07}, SpanID: adapters.SpanID{0x06}},
		},
		Status:                adapters.StatusError,
		StatusDesc:            "bad",
		InstrumentationScope:  &adapters.InstrumentationScope{Name: "lib", Version: "v1"},
		DroppedAttributeCount: 1,
		DroppedEventCount:     2,
		DroppedLinkCount:      3,
	}
	require.NoError(t, a.ExportSpans(t.Context(), []adapters.SpanData{span}))

	req := receive(t, col.got)
	require.Len(t, req.GetResourceSpans(), 1)
	rs := req.GetResourceSpans()[0]

	res := attrMap(rs.GetResource().GetAttributes())
	require.Equal(t, "svc", res["service.name"].GetStringValue())
	require.Equal(t, "1.2.3", res["service.version"].GetStringValue())
	require.Equal(t, "prod", res["deployment.environment"].GetStringValue())
	require.Equal(t, "eu", res["region"].GetStringValue())

	require.Len(t, rs.GetScopeSpans(), 1)
	ss := rs.GetScopeSpans()[0]
	require.Equal(t, "lib", ss.GetScope().GetName())
	require.Equal(t, "v1", ss.GetScope().GetVersion())
	require.Len(t, ss.GetSpans(), 1)

	pb := ss.GetSpans()[0]
	require.Equal(t, span.TraceID[:], pb.GetTraceId())
	require.Equal(t, span.SpanID[:], pb.GetSpanId())
	require.Equal(t, span.ParentID[:], pb.GetParentSpanId())
	require.Equal(t, "op", pb.GetName())
	require.Equal(t, tracepb.Span_SPAN_KIND_SERVER, pb.GetKind())
	require.Equal(t, uint64(start.UnixNano()), pb.GetStartTimeUnixNano())
	require.Equal(t, uint64(end.UnixNano()), pb.GetEndTimeUnixNano())
	require.Equal(t, uint32(1), pb.GetDroppedAttributesCount())
	require.Equal(t, uint32(2), pb.GetDroppedEventsCount())
	require.Equal(t, uint32(3), pb.GetDroppedLinksCount())
	require.Equal(t, tracepb.Status_STATUS_CODE_ERROR, pb.GetStatus().GetCode())
	require.Equal(t, "bad", pb.GetStatus().GetMessage())

	attrs := attrMap(pb.GetAttributes())
	require.Equal(t, "str", attrs["s"].GetStringValue())
	require.Equal(t, int64(7), attrs["i"].GetIntValue())
	require.Equal(t, int64(8), attrs["i64"].GetIntValue())
	require.InDelta(t, 1.5, attrs["f"].GetDoubleValue(), 0)
	require.True(t, attrs["b"].GetBoolValue())
	require.Equal(t, "custom", attrs["other"].GetStringValue())

	arr := func(k string) []*commonpb.AnyValue { return attrs[k].GetArrayValue().GetValues() }
	require.Len(t, arr("ss"), 2)
	require.Equal(t, "b", arr("ss")[1].GetStringValue())
	require.Equal(t, int64(2), arr("is")[1].GetIntValue())
	require.Equal(t, int64(3), arr("i64s")[0].GetIntValue())
	require.InDelta(t, 0.5, arr("fs")[0].GetDoubleValue(), 0)
	require.False(t, arr("bs")[1].GetBoolValue())
	require.True(t, arr("bs")[0].GetBoolValue())

	require.Len(t, pb.GetEvents(), 2)
	require.Equal(t, "ev", pb.GetEvents()[0].GetName())
	require.Equal(t, uint64(start.UnixNano()), pb.GetEvents()[0].GetTimeUnixNano())
	require.Equal(t, "ev", attrMap(pb.GetEvents()[0].GetAttributes())["ek"].GetStringValue())
	require.Empty(t, pb.GetEvents()[1].GetAttributes())

	require.Len(t, pb.GetLinks(), 2)
	require.Equal(t, []byte{0x09, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, pb.GetLinks()[0].GetTraceId())
	require.Equal(t, "k=v", pb.GetLinks()[0].GetTraceState())
	require.False(t, attrMap(pb.GetLinks()[0].GetAttributes())["lk"].GetBoolValue())
	require.Empty(t, pb.GetLinks()[1].GetAttributes())

	require.NoError(t, a.ForceFlush(t.Context()))
	require.NoError(t, a.Shutdown(t.Context()))
}

func TestExportSpans_KindsStatusesAndScopes(t *testing.T) {
	t.Parallel()
	col, conn := startRequestCollector(t, nil)

	a, err := otlp.New(t.Context(), otlp.WithGRPCConn(conn))
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	kinds := map[adapters.SpanKind]tracepb.Span_SpanKind{
		adapters.SpanKindUnspecified: tracepb.Span_SPAN_KIND_UNSPECIFIED,
		adapters.SpanKindInternal:    tracepb.Span_SPAN_KIND_INTERNAL,
		adapters.SpanKindServer:      tracepb.Span_SPAN_KIND_SERVER,
		adapters.SpanKindClient:      tracepb.Span_SPAN_KIND_CLIENT,
		adapters.SpanKindProducer:    tracepb.Span_SPAN_KIND_PRODUCER,
		adapters.SpanKindConsumer:    tracepb.Span_SPAN_KIND_CONSUMER,
	}
	statuses := []adapters.StatusCode{adapters.StatusUnset, adapters.StatusOK, adapters.StatusError, adapters.StatusOK, adapters.StatusUnset, adapters.StatusError}
	wantStatus := map[adapters.StatusCode]tracepb.Status_StatusCode{
		adapters.StatusUnset: tracepb.Status_STATUS_CODE_UNSET,
		adapters.StatusOK:    tracepb.Status_STATUS_CODE_OK,
		adapters.StatusError: tracepb.Status_STATUS_CODE_ERROR,
	}

	spans := make([]adapters.SpanData, 0, len(kinds))
	for k := range adapters.SpanKindConsumer + 1 {
		s := adapters.SpanData{Name: k.String(), Kind: k, Status: statuses[k], SpanID: adapters.SpanID{byte(k + 1)}}
		if k%2 == 1 {
			s.InstrumentationScope = &adapters.InstrumentationScope{Name: "odd"}
		}
		spans = append(spans, s)
	}
	require.NoError(t, a.ExportSpans(t.Context(), spans))

	req := receive(t, col.got)
	scopes := req.GetResourceSpans()[0].GetScopeSpans()
	require.Len(t, scopes, 2, "spans are grouped by instrumentation scope")

	seen := 0
	for _, ss := range scopes {
		for _, pb := range ss.GetSpans() {
			k := adapters.SpanKind(pb.GetSpanId()[0] - 1)
			require.Equal(t, kinds[k], pb.GetKind(), pb.GetName())
			require.Equal(t, wantStatus[statuses[k]], pb.GetStatus().GetCode(), pb.GetName())
			require.Empty(t, pb.GetParentSpanId())
			if k%2 == 1 {
				require.Equal(t, "odd", ss.GetScope().GetName())
			} else {
				require.Nil(t, ss.GetScope())
			}
			seen++
		}
	}
	require.Equal(t, len(spans), seen)
}

func TestExportSpans_EmptyBatchSkipsUpload(t *testing.T) {
	t.Parallel()
	col, conn := startRequestCollector(t, nil)

	a, err := otlp.New(t.Context(), otlp.WithGRPCConn(conn))
	require.NoError(t, err)
	require.NoError(t, a.ExportSpans(t.Context(), nil))
	require.NoError(t, a.Shutdown(t.Context()))
	require.Empty(t, col.got)
}

func TestExportSpans_AfterShutdownFails(t *testing.T) {
	t.Parallel()
	_, conn := startRequestCollector(t, nil)

	a, err := otlp.New(t.Context(), otlp.WithGRPCConn(conn))
	require.NoError(t, err)
	require.NoError(t, a.Shutdown(t.Context()))
	require.NoError(t, a.Shutdown(t.Context()), "second Shutdown is a no-op")
	require.NoError(t, a.ForceFlush(t.Context()), "ForceFlush after Shutdown is a no-op")
	require.Error(t, a.ExportSpans(t.Context(), testSpans()))
}

func TestExportSpans_CollectorErrorPropagates(t *testing.T) {
	t.Parallel()
	_, conn := startRequestCollector(t, status.Error(codes.Unavailable, "down"))

	a, err := otlp.New(t.Context(), otlp.WithGRPCConn(conn), otlp.WithCompression(true))
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	err = a.ExportSpans(t.Context(), testSpans())
	require.Error(t, err)
	require.Equal(t, codes.Unavailable, status.Code(errors.Unwrap(err)))
}

func TestNew_HTTPProtocolExportsWithHeadersAndCompression(t *testing.T) {
	t.Parallel()
	type got struct {
		path, apiKey, encoding string
	}
	reqs := make(chan got, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs <- got{r.URL.Path, r.Header.Get("X-Api-Key"), r.Header.Get("Content-Encoding")}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	a, err := otlp.New(t.Context(),
		otlp.WithProtocol(otlp.ProtocolHTTP),
		otlp.WithEndpoint(strings.TrimPrefix(srv.URL, "http://")),
		otlp.WithInsecure(),
		otlp.WithHeaders(map[string]string{"X-Api-Key": "k"}),
		otlp.WithCompression(true),
	)
	require.NoError(t, err)
	require.NoError(t, a.ExportSpans(t.Context(), testSpans()))

	r := receive(t, reqs)
	require.Equal(t, "/v1/traces", r.path)
	require.Equal(t, "k", r.apiKey)
	require.Equal(t, "gzip", r.encoding)
	require.NoError(t, a.Shutdown(t.Context()))
}

func TestStringOptions_IgnoreBlankAndNil(t *testing.T) {
	t.Parallel()
	col, conn := startRequestCollector(t, nil)

	var nilStr *string
	blank := "  "
	ver := " 2.0 "
	a, err := otlp.New(t.Context(),
		otlp.WithGRPCConn(conn),
		otlp.WithServiceName(" "),
		otlp.WithServiceName(nilStr),
		otlp.WithServiceName(&blank),
		otlp.WithServiceVersion(&ver),
		otlp.WithServiceVersion(nilStr),
		otlp.WithServiceVersion(""),
		otlp.WithEnvironment(&blank),
		otlp.WithEnvironment(nilStr),
		otlp.WithEndpoint(nilStr),
		otlp.WithEndpoint(&blank),
		otlp.WithExportTimeout(-time.Second),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	require.NoError(t, a.ExportSpans(t.Context(), testSpans()))

	res := attrMap(receive(t, col.got).GetResourceSpans()[0].GetResource().GetAttributes())
	require.Equal(t, otlp.DefaultServiceName, res["service.name"].GetStringValue())
	require.Equal(t, "2.0", res["service.version"].GetStringValue())
	require.NotContains(t, res, "deployment.environment")
}

func TestPointerStringOptions_Apply(t *testing.T) {
	t.Parallel()
	col, conn := startRequestCollector(t, nil)

	name, env := "psvc", "staging"
	a, err := otlp.New(t.Context(),
		otlp.WithGRPCConn(conn),
		otlp.WithServiceName(&name),
		otlp.WithEnvironment(&env),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	require.NoError(t, a.ExportSpans(t.Context(), testSpans()))

	res := attrMap(receive(t, col.got).GetResourceSpans()[0].GetResource().GetAttributes())
	require.Equal(t, "psvc", res["service.name"].GetStringValue())
	require.Equal(t, "staging", res["deployment.environment"].GetStringValue())
	require.NotContains(t, res, "service.version")
}
