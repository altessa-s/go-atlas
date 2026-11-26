// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/otlp"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// unreachableEndpoint is a collector address nothing listens on: an export
// that succeeds while the adapter is configured with it went through the
// injected client.
const unreachableEndpoint = "127.0.0.1:1"

// traceCollector is an in-process OTLP/gRPC collector recording the
// metadata of every Export call.
type traceCollector struct {
	coltracepb.UnimplementedTraceServiceServer
	got      chan metadata.MD
	rejected int64
}

func (c *traceCollector) Export(ctx context.Context, _ *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	c.got <- md
	if c.rejected > 0 {
		return &coltracepb.ExportTraceServiceResponse{
			PartialSuccess: &coltracepb.ExportTracePartialSuccess{
				RejectedSpans: c.rejected,
				ErrorMessage:  "dropped invalid span",
			},
		}, nil
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func startCollector(t *testing.T) (string, *traceCollector) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	col := &traceCollector{got: make(chan metadata.MD, 8)}
	srv := grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(srv, col)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String(), col
}

func testSpans() []adapters.SpanData {
	end := time.Now()
	return []adapters.SpanData{{
		TraceID:   adapters.TraceID{0x01},
		SpanID:    adapters.SpanID{0x02},
		Name:      "op",
		StartTime: end.Add(-time.Millisecond),
		EndTime:   end,
	}}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(10 * time.Second):
		t.Fatal("no OTLP export received")
		var zero T
		return zero
	}
}

// An injected gRPC connection carries the export and stays owned by the
// caller: Shutdown leaves it open.
func TestNew_UsesInjectedGRPCConn(t *testing.T) {
	t.Parallel()
	addr, col := startCollector(t)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	a, err := otlp.New(t.Context(),
		otlp.WithEndpoint(unreachableEndpoint),
		otlp.WithGRPCConn(conn),
		otlp.WithHeaders(map[string]string{"X-Api-Key": "secret"}),
	)
	require.NoError(t, err)
	res, err := a.ExportSpans(t.Context(), testSpans())
	require.NoError(t, err)
	require.Equal(t, adapters.ExportResult{Accepted: 1}, res)

	md := receive(t, col.got)
	require.Equal(t, []string{"secret"}, md.Get("x-api-key"))

	require.NoError(t, a.Shutdown(t.Context()))
	require.NotEqual(t, connectivity.Shutdown, conn.GetState(), "Shutdown must not close a caller-owned connection")
}

// Without an injected connection the adapter dials the endpoint itself.
func TestNew_DefaultGRPCClient(t *testing.T) {
	t.Parallel()
	addr, col := startCollector(t)

	a, err := otlp.New(t.Context(), otlp.WithEndpoint(addr), otlp.WithInsecure())
	require.NoError(t, err)
	res, err := a.ExportSpans(t.Context(), testSpans())
	require.NoError(t, err)
	require.Equal(t, adapters.ExportResult{Accepted: 1}, res)
	receive(t, col.got)
	require.NoError(t, a.Shutdown(t.Context()))
}

// An injected *http.Client sends the HTTP export.
func TestNew_UsesInjectedHTTPClient(t *testing.T) {
	t.Parallel()
	paths := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	var calls atomic.Int32
	target := strings.TrimPrefix(srv.URL, "http://")
	hc := &http.Client{Transport: testhelpers.RoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		r = r.Clone(r.Context())
		r.URL.Host = target
		return http.DefaultTransport.RoundTrip(r)
	})}

	a, err := otlp.New(t.Context(),
		otlp.WithProtocol(otlp.ProtocolHTTP),
		otlp.WithEndpoint(unreachableEndpoint),
		otlp.WithInsecure(),
		otlp.WithHTTPClient(hc),
	)
	require.NoError(t, err)
	res, err := a.ExportSpans(t.Context(), testSpans())
	require.NoError(t, err)
	require.Equal(t, adapters.ExportResult{Accepted: 1}, res)

	require.Equal(t, "/v1/traces", receive(t, paths))
	require.Positive(t, calls.Load())
	require.NoError(t, a.Shutdown(t.Context()))
}

func TestForceFlush_StripsCallerOutgoingMetadata(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		headers map[string]string
	}{
		{name: "with exporter headers", headers: map[string]string{"X-Api-Key": "exporter-secret"}},
		{name: "without exporter headers", headers: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			addr, col := startCollector(t)
			conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			require.NoError(t, err)
			t.Cleanup(func() { _ = conn.Close() })

			opts := []otlp.Option{otlp.WithGRPCConn(conn)}
			if len(tc.headers) > 0 {
				opts = append(opts, otlp.WithHeaders(tc.headers))
			}
			a, err := otlp.New(t.Context(), opts...)
			require.NoError(t, err)
			tr := tracing.New(tracing.WithAdapter(a), tracing.WithScheduledDelay(time.Hour))
			defer func() { require.NoError(t, tr.Shutdown(t.Context())) }()

			_, span := tr.Recorder("svc").Start(t.Context(), "op")
			span.End()

			flushCtx := metadata.NewOutgoingContext(t.Context(), metadata.Pairs("authorization", "Bearer caller-secret"))
			flushCtx = metadata.AppendToOutgoingContext(flushCtx, "x-session-token", "session-secret")
			require.NoError(t, tr.ForceFlush(flushCtx))

			md := receive(t, col.got)
			require.Empty(t, md.Get("authorization"))
			require.Empty(t, md.Get("x-session-token"))
			if len(tc.headers) > 0 {
				require.Equal(t, []string{"exporter-secret"}, md.Get("x-api-key"))
			}
		})
	}
}

func TestTracer_PartialSuccessCountsRejectedSpans(t *testing.T) {
	t.Parallel()
	addr, col := startCollector(t)
	col.rejected = 1

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	a, err := otlp.New(t.Context(), otlp.WithGRPCConn(conn))
	require.NoError(t, err)

	meter := metricstest.NewTestCollector()
	tr := tracing.New(tracing.WithAdapter(a), tracing.WithMeter(meter), tracing.WithScheduledDelay(time.Hour))
	defer func() { require.NoError(t, tr.Shutdown(t.Context())) }()

	rec := tr.Recorder("svc")
	for range 2 {
		_, span := rec.Start(t.Context(), "op")
		span.End()
	}
	require.NoError(t, tr.ForceFlush(t.Context()))
	receive(t, col.got)

	require.Equal(t, 1.0, meter.CounterValue("test_tracing_spans_exported_total", nil))
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, meter, "test_tracing_spans_rejected_total", "exporter", "otlp"))
}

func TestTracer_MultiExporterPartialSuccessCounters(t *testing.T) {
	t.Parallel()
	addr, col := startCollector(t)
	col.rejected = 1

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	otlpExp, err := otlp.New(t.Context(), otlp.WithGRPCConn(conn))
	require.NoError(t, err)
	consoleExp := console.New(console.WithWriter(io.Discard))

	meter := metricstest.NewTestCollector()
	tr := tracing.New(
		tracing.WithAdapter(adapters.NewMultiAdapter(consoleExp, otlpExp)),
		tracing.WithMeter(meter),
		tracing.WithScheduledDelay(time.Hour),
	)
	defer func() { require.NoError(t, tr.Shutdown(t.Context())) }()

	rec := tr.Recorder("svc")
	for range 2 {
		_, span := rec.Start(t.Context(), "op")
		span.End()
	}
	require.NoError(t, tr.ForceFlush(t.Context()))
	receive(t, col.got)

	require.Equal(t, 1.0, meter.CounterValue("test_tracing_spans_exported_total", nil))
	require.Equal(t, 1.0, metricstest.GetCounterValue(t, meter, "test_tracing_spans_rejected_total", "exporter", "otlp"))
	require.Equal(t, 0.0, metricstest.GetCounterValue(t, meter, "test_tracing_spans_rejected_total", "exporter", "console"))
}
