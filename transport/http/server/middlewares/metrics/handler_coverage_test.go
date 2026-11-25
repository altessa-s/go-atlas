// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metrics_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/metrics"
)

func writeStatus(code int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	})
}

func TestName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "metrics", metrics.Name())
}

func TestHandler_SizeMetrics(t *testing.T) {
	t.Parallel()

	col := metricstest.NewTestCollector()
	h := metrics.New(metrics.WithMeter(col), metrics.WithEnableSizeMetrics()).Handler(writeStatus(http.StatusCreated, "created!"))

	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader("payload"))
	h.ServeHTTP(httptest.NewRecorder(), req)

	// Unknown length (-1) is not observed as a request size.
	req = httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader("x"))
	req.ContentLength = -1
	h.ServeHTTP(httptest.NewRecorder(), req)

	require.Equal(t, float64(2), metricstest.GetCounterValue(t, col, "test_http_server_requests_total", "method", "POST", "status", "201", "route", "/_unknown"))
	require.Equal(t, uint64(1), metricstest.GetHistogramCount(t, col, "test_http_server_request_size_bytes", "method", "POST", "status", "201", "route", "/_unknown"))
	require.Equal(t, uint64(2), metricstest.GetHistogramCount(t, col, "test_http_server_response_size_bytes", "method", "POST", "status", "201", "route", "/_unknown"))
	require.Equal(t, float64(0), metricstest.GetGaugeValue(t, col, "test_http_server_requests_in_flight"))
}

func TestHandler_NonStandardStatusCode(t *testing.T) {
	t.Parallel()

	col := metricstest.NewTestCollector()
	h := metrics.New(metrics.WithMeter(col)).Handler(writeStatus(799, ""))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/x", nil))
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, "test_http_server_requests_total", "method", "GET", "status", "799", "route", "/_unknown"))
}

func TestHandler_IgnoredPaths(t *testing.T) {
	t.Parallel()

	col := metricstest.NewTestCollector()
	var logs bytes.Buffer
	h := metrics.New(
		metrics.WithMeter(col),
		metrics.WithIgnorePaths("/healthz"),
		metrics.WithIgnorePatterns(regexp.MustCompile(`^/debug/`)),
		metrics.WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))),
	).Handler(writeStatus(http.StatusOK, "ok"))

	for _, path := range []string{"/healthz", "/debug/pprof"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, rec.Code)
	}
	require.Zero(t, metricstest.GetCounterValue(t, col, "test_http_server_requests_total", "method", "GET", "status", "200", "route", "/_unknown"))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api", nil))
	require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, "test_http_server_requests_total", "method", "GET", "status", "200", "route", "/_unknown"))
	require.Contains(t, logs.String(), "recorded prometheus metrics")
}

func TestWithMetricsSubsystem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opt  metrics.Option
		want string
	}{
		{name: "string", opt: metrics.WithMetricsSubsystem(" public "), want: "test_public_server_requests_total"},
		{name: "pointer", opt: metrics.WithMetricsSubsystem(new("admin")), want: "test_admin_server_requests_total"},
		{name: "nil pointer", opt: metrics.WithMetricsSubsystem((*string)(nil)), want: "test_http_server_requests_total"},
		{name: "blank pointer", opt: metrics.WithMetricsSubsystem(new(" ")), want: "test_http_server_requests_total"},
		{name: "blank string", opt: metrics.WithMetricsSubsystem(""), want: "test_http_server_requests_total"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			col := metricstest.NewTestCollector()
			metrics.New(metrics.WithMeter(col), tc.opt).Handler(writeStatus(http.StatusOK, "")).
				ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
			require.Equal(t, float64(1), metricstest.GetCounterValue(t, col, tc.want, "method", "GET", "status", "200", "route", "/_unknown"))
		})
	}
}
