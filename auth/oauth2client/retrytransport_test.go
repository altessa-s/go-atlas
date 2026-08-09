// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
		oauth2client.WithHTTPClient(client))

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

// TestRetryTransportContextCanceledDuringBackoff pins that a canceled context
// aborts the backoff instead of sleeping it out.
//
// This test used to be flaky (roughly one run in forty) for a reason worth
// recording: the request carried http.NoBody, which http.NewRequest represents
// as a non-nil Body with a nil GetBody. The transport read that as "cannot be
// replayed" and took the single-attempt path, so no backoff ever happened and
// the context.Canceled being asserted was whatever net/http happened to report
// while racing the handler's cancel() — nil whenever the response won. The
// transport now recognizes http.NoBody as replayable, which is what makes the
// retry path, and therefore this assertion, actually run.
func TestRetryTransportContextCanceledDuringBackoff(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)

		// Abort while RoundTrip backs off before the retry.
		//
		// Waiting for the cancellation to land before returning is what makes
		// this deterministic: otherwise the client can observe the whole
		// response and move on while this goroutine has not reached cancel()
		// yet, and the retry loop then proceeds as if nothing was canceled.
		// ctx is the one cancel() closes, so the receive returns immediately.
		cancel()
		<-ctx.Done()
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

// TestRetryTransportRetriesBodylessRequests pins which requests are eligible
// for a retry at all.
//
// http.NewRequest represents a nil body and http.NoBody differently — the
// second leaves a non-nil Body with a nil GetBody — and both mean "there is
// nothing to send". A transport that only accepts "GetBody != nil" therefore
// gives up retries on the most idiomatic way of writing an empty request,
// silently: the caller sees one attempt and a 503, with no indication that the
// retry policy it configured was never consulted.
func TestRetryTransportRetriesBodylessRequests(t *testing.T) {
	t.Parallel()

	bodies := map[string]io.Reader{
		"nil body":     nil,
		"http.NoBody":  http.NoBody,
		"bytes.Reader": bytes.NewReader([]byte("payload")),
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			t.Cleanup(srv.Close)

			client := &http.Client{Transport: oauth2client.RetryTransport(http.DefaultTransport,
				oauth2client.WithTransportAttempts(2),
				oauth2client.WithTransportBackoff(time.Millisecond, 2*time.Millisecond),
			)}

			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, body)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			t.Cleanup(func() { _ = resp.Body.Close() })

			require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			require.Equal(t, int32(3), hits.Load(), "the request was not retried") // 1 + 2 retries
		})
	}
}
