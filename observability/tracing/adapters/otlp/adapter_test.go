// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters"
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
	got chan metadata.MD
}

func (c *traceCollector) Export(ctx context.Context, _ *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	c.got <- md
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
	require.NoError(t, a.ExportSpans(t.Context(), testSpans()))

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
	require.NoError(t, a.ExportSpans(t.Context(), testSpans()))
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
	require.NoError(t, a.ExportSpans(t.Context(), testSpans()))

	require.Equal(t, "/v1/traces", receive(t, paths))
	require.Positive(t, calls.Load())
	require.NoError(t, a.Shutdown(t.Context()))
}
