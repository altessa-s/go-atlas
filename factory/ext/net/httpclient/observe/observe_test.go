// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package observe_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/httpx"
	"github.com/altessa-s/go-atlas/factory/ext/net/httpclient/observe"
	"github.com/altessa-s/go-atlas/observability/metrics/metricstest"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
)

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func observedClient(obs httpclient.Observer, opts ...httpclient.Option) *http.Client {
	return httpclient.New(append([]httpclient.Option{
		httpclient.WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8")),
		httpclient.WithObserver(obs),
		httpclient.WithRetryMax(0),
	}, opts...)...)
}

func get(t *testing.T, c *http.Client, url string) error {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	return err
}

// TestMetricsSubsystem_DefaultRegistersUnderHTTPClient confirms that omitting
// WithMetricsSubsystem keeps the "http_client" namespace so existing
// dashboards stay valid.
func TestMetricsSubsystem_DefaultRegistersUnderHTTPClient(t *testing.T) {
	t.Parallel()
	srv := okServer(t)

	tc := metricstest.NewTestCollector()
	c := observedClient(observe.New(tc, nil))
	require.NoError(t, get(t, c, srv.URL))

	val := metricstest.GetCounterValue(t, tc, "test_http_client_requests_total",
		"method", "GET", "status_class", "2xx")
	require.Equal(t, float64(1), val)
	require.Equal(t, uint64(1), metricstest.GetHistogramCount(t, tc, "test_http_client_request_duration_seconds",
		"method", "GET"))
}

// TestMetricsSubsystem_CustomRoutesMetricsToOwnNamespace confirms the
// motivating use case: a service with multiple HTTP clients (egrul, kfocus,
// ...) can scope their metrics independently by passing WithMetricsSubsystem.
func TestMetricsSubsystem_CustomRoutesMetricsToOwnNamespace(t *testing.T) {
	t.Parallel()
	srv := okServer(t)

	tc := metricstest.NewTestCollector()
	c := observedClient(observe.New(tc, nil, observe.WithMetricsSubsystem("egrul")))
	require.NoError(t, get(t, c, srv.URL))

	val := metricstest.GetCounterValue(t, tc, "test_egrul_requests_total",
		"method", "GET", "status_class", "2xx")
	require.Equal(t, float64(1), val)

	require.False(t, metricstest.GatherMetric(t, tc, "test_http_client_requests_total"),
		"default subsystem must not appear when a custom one is set")
}

// TestMetricsSubsystem_TwoClientsShareRegistry pins the contract that two
// HTTP clients with distinct subsystems can share the same collector
// without panicking on duplicate metric registration.
func TestMetricsSubsystem_TwoClientsShareRegistry(t *testing.T) {
	t.Parallel()
	srv := okServer(t)

	tc := metricstest.NewTestCollector()

	require.NotPanics(t, func() {
		egrul := observedClient(observe.New(tc, nil, observe.WithMetricsSubsystem("egrul")))
		kfocus := observedClient(observe.New(tc, nil, observe.WithMetricsSubsystem("kfocus")))
		require.NoError(t, get(t, egrul, srv.URL))
		require.NoError(t, get(t, kfocus, srv.URL))
	})

	require.Equal(t, float64(1),
		metricstest.GetCounterValue(t, tc, "test_egrul_requests_total",
			"method", "GET", "status_class", "2xx"))
	require.Equal(t, float64(1),
		metricstest.GetCounterValue(t, tc, "test_kfocus_requests_total",
			"method", "GET", "status_class", "2xx"))
}

func TestMetrics_RetriesErrorsAndBreaker(t *testing.T) {
	t.Parallel()

	tc := metricstest.NewTestCollector()
	c := observedClient(observe.New(tc, nil),
		httpclient.WithoutSSRFProtection(),
		httpclient.WithClient(&http.Client{Transport: httpx.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("forced failure")
		})}),
		httpclient.WithRetryMax(1),
		httpclient.WithRetryWait(time.Millisecond, time.Millisecond),
		httpclient.WithBreakerTimeout(time.Hour),
	)

	// Request 1: two failed attempts. Request 2: its first attempt trips the
	// breaker (three failures), its retry is rejected. Request 3: rejected.
	for range 3 {
		require.Error(t, get(t, c, "http://down.test/"))
	}

	require.Equal(t, float64(3), metricstest.GetCounterValue(t, tc, "test_http_client_request_errors_total", "method", "GET"))
	require.Equal(t, float64(3), metricstest.GetCounterValue(t, tc, "test_http_client_requests_total",
		"method", "GET", "status_class", "unknown"))
	require.Equal(t, float64(2), metricstest.GetCounterValue(t, tc, "test_http_client_retries_total"))
	require.Equal(t, float64(2), metricstest.GetCounterValue(t, tc, "test_http_client_circuit_breaker_trips_total"))
	require.Equal(t, float64(2), metricstest.GetGaugeValue(t, tc, "test_http_client_circuit_breaker_state", "host", "down.test"))
}
