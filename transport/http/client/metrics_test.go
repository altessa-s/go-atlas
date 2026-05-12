// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// TestMetricsSubsystem_DefaultRegistersUnderHTTPClient confirms that omitting
// WithMetricsSubsystem keeps the legacy "http_client" namespace so existing
// dashboards stay valid.
func TestMetricsSubsystem_DefaultRegistersUnderHTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tc := testhelpers.NewTestCollector()
	c := httpclient.New(
		httpclient.WithCollector(tc),
		httpclient.WithRetryMax(0),
	)

	resp, err := c.Get(srv.URL)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	val := testhelpers.GetCounterValue(t, tc, "test_http_client_requests_total",
		"method", "GET", "status_class", "2xx")
	require.Equal(t, float64(1), val)
}

// TestMetricsSubsystem_CustomRoutesMetricsToOwnNamespace confirms the
// motivating use case: a service with multiple HTTP clients (egrul, kfocus,
// ...) can scope their metrics independently by passing WithMetricsSubsystem.
func TestMetricsSubsystem_CustomRoutesMetricsToOwnNamespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tc := testhelpers.NewTestCollector()
	c := httpclient.New(
		httpclient.WithCollector(tc),
		httpclient.WithMetricsSubsystem("egrul"),
		httpclient.WithRetryMax(0),
	)

	resp, err := c.Get(srv.URL)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	val := testhelpers.GetCounterValue(t, tc, "test_egrul_requests_total",
		"method", "GET", "status_class", "2xx")
	require.Equal(t, float64(1), val)

	require.False(t, testhelpers.GatherMetric(t, tc, "test_http_client_requests_total"),
		"default subsystem must not appear when a custom one is set")
}

// TestMetricsSubsystem_TwoClientsShareRegistry pins the contract that two
// HTTP clients with distinct subsystems can share the same collector
// without panicking on duplicate metric registration — the bug that
// motivated this option.
func TestMetricsSubsystem_TwoClientsShareRegistry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tc := testhelpers.NewTestCollector()

	require.NotPanics(t, func() {
		egrul := httpclient.New(
			httpclient.WithCollector(tc),
			httpclient.WithMetricsSubsystem("egrul"),
			httpclient.WithRetryMax(0),
		)
		kfocus := httpclient.New(
			httpclient.WithCollector(tc),
			httpclient.WithMetricsSubsystem("kfocus"),
			httpclient.WithRetryMax(0),
		)

		respE, err := egrul.Get(srv.URL)
		require.NoError(t, err)
		require.NoError(t, respE.Body.Close())

		respK, err := kfocus.Get(srv.URL)
		require.NoError(t, err)
		require.NoError(t, respK.Body.Close())
	})

	require.Equal(t, float64(1),
		testhelpers.GetCounterValue(t, tc, "test_egrul_requests_total",
			"method", "GET", "status_class", "2xx"))
	require.Equal(t, float64(1),
		testhelpers.GetCounterValue(t, tc, "test_kfocus_requests_total",
			"method", "GET", "status_class", "2xx"))
}
