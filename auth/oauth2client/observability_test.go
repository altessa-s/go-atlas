// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

const (
	fetchesMetric  = "test_auth_oauth2client_token_fetches_total"
	durationMetric = "test_auth_oauth2client_token_fetch_duration_seconds"
	retriesMetric  = "test_auth_oauth2client_token_fetch_retries_total"
)

func TestClientCredentialsMetrics(t *testing.T) {
	t.Parallel()
	tc := testhelpers.NewTestCollector()
	m := oauth2client.NewMetrics(tc, "")
	srv, _ := tokenServer(t, "cc-token")

	src := oauth2client.ClientCredentials(t.Context(), srv.URL, "svc", "sec", oauth2client.WithMetrics(m))
	_, err := src.Token()
	require.NoError(t, err)
	// A second call is served from the reuse cache and must not be counted.
	_, err = src.Token()
	require.NoError(t, err)

	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, fetchesMetric,
		"grant", "client_credentials", "status", "success"))
	require.Equal(t, uint64(1), testhelpers.GetHistogramCount(t, tc, durationMetric,
		"grant", "client_credentials", "status", "success"))
}

func TestExchangerMetricsAndRetry(t *testing.T) {
	t.Parallel()
	tc := testhelpers.NewTestCollector()
	m := oauth2client.NewMetrics(tc, "")

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"access_token": "ok", "token_type": "Bearer"}))
	}))
	t.Cleanup(srv.Close)

	ex := oauth2client.NewExchanger(srv.URL, "svc", "sec",
		oauth2client.WithMetrics(m),
		oauth2client.WithRetryAttempts(5),
		oauth2client.WithRetryBaseDelay(time.Millisecond),
		oauth2client.WithRetryMaxDelay(5*time.Millisecond),
	)
	_, err := ex.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subj"})
	require.NoError(t, err)

	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, fetchesMetric,
		"grant", "token_exchange", "status", "success"))
	require.Equal(t, 2.0, testhelpers.GetCounterValue(t, tc, retriesMetric, "grant", "token_exchange"))
}

func TestExchangerMetricsFailureAndLog(t *testing.T) {
	t.Parallel()
	tc := testhelpers.NewTestCollector()
	m := oauth2client.NewMetrics(tc, "")

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	ex := oauth2client.NewExchanger(srv.URL, "svc", "sec",
		oauth2client.WithMetrics(m), oauth2client.WithLogger(logger))
	_, err := ex.Exchange(t.Context(), oauth2client.ExchangeRequest{SubjectToken: "subj"})
	require.ErrorIs(t, err, oauth2client.ErrTokenExchange)

	require.Equal(t, 1.0, testhelpers.GetCounterValue(t, tc, fetchesMetric,
		"grant", "token_exchange", "status", "error"))
	require.Contains(t, buf.String(), "token exchange failed")
}
