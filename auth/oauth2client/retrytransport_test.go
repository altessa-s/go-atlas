// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oauth2client"
)

func TestRetryTransportOnClientCredentials(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"access_token": "cc", "token_type": "Bearer", "expires_in": 3600,
		}))
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{Transport: oauth2client.RetryTransport(http.DefaultTransport,
		oauth2client.WithTransportAttempts(5),
		oauth2client.WithTransportBackoff(time.Millisecond, 5*time.Millisecond),
	)}
	src := oauth2client.ClientCredentials(t.Context(), srv.URL, "svc", "sec",
		oauth2client.WithHttpClient(client))

	tok, err := src.Token()
	require.NoError(t, err)
	require.Equal(t, "cc", tok.AccessToken)
	require.Equal(t, int32(3), hits.Load()) // 2 failures + 1 success
}

func TestRetryTransportStatusHandling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   int
		attempts int
		wantHits int32
	}{
		// The last response is surfaced after retries are exhausted: 1 + 2 retries.
		{"gives up and returns last response", http.StatusBadGateway, 2, 3},
		// 4xx is not retried.
		{"no retry on client error", http.StatusBadRequest, 5, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(tt.status)
			}))
			t.Cleanup(srv.Close)

			client := &http.Client{Transport: oauth2client.RetryTransport(http.DefaultTransport,
				oauth2client.WithTransportAttempts(tt.attempts),
				oauth2client.WithTransportBackoff(time.Millisecond, 5*time.Millisecond),
			)}
			resp, err := client.Get(srv.URL)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })
			require.Equal(t, tt.status, resp.StatusCode)
			require.Equal(t, tt.wantHits, hits.Load())
		})
	}
}

func TestRetryTransportContextCanceledDuringBackoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		cancel() // abort while RoundTrip backs off before the retry
	}))
	t.Cleanup(srv.Close)

	// A backoff far above the test deadline: only prompt cancellation passes.
	client := &http.Client{Transport: oauth2client.RetryTransport(http.DefaultTransport,
		oauth2client.WithTransportAttempts(3),
		oauth2client.WithTransportBackoff(time.Minute, time.Minute),
	)}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
	require.NoError(t, err)

	start := time.Now()
	resp, err := client.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(start), 30*time.Second) // aborted mid-backoff, not after it
	require.Equal(t, int32(1), hits.Load())
}
