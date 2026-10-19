// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

// failingServer answers 503 to the first failures requests and 200 after.
func failingServer(t *testing.T, failures int32) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) <= failures {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// get sends one GET through c.
func get(t *testing.T, c *http.Client, url string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
	return resp, err
}

func TestWithLimiterPerAttempt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		perAttempt bool
		wantCalls  int32
	}{
		{"around the retry loop", false, 1},
		{"per attempt", true, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, hits := failingServer(t, 2)
			var calls atomic.Int32
			opts := []httpclient.Option{
				loopback, httpclient.WithRetryMax(3), httpclient.WithRetryWait(time.Millisecond, time.Millisecond),
				httpclient.WithLimiter(limiters.Func(func(context.Context, string) error { calls.Add(1); return nil })),
			}
			if tc.perAttempt {
				opts = append(opts, httpclient.WithLimiterPerAttempt())
			}
			resp, err := get(t, httpclient.New(opts...), srv.URL)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, int32(3), hits.Load())
			require.Equal(t, tc.wantCalls, calls.Load())
		})
	}
}

func TestWithLimiterPerAttempt_RefusalNotRetried(t *testing.T) {
	t.Parallel()

	srv, hits := failingServer(t, 0)
	errRefused := errors.New("budget exhausted")
	var calls atomic.Int32
	c := httpclient.New(loopback, httpclient.WithRetryMax(3), httpclient.WithRetryWait(time.Millisecond, time.Millisecond),
		httpclient.WithLimiterPerAttempt(),
		httpclient.WithLimiter(limiters.Func(func(context.Context, string) error { calls.Add(1); return errRefused })))

	_, err := get(t, c, srv.URL)
	require.ErrorIs(t, err, limiters.ErrRateLimited)
	require.ErrorIs(t, err, errRefused)
	require.Equal(t, int32(1), calls.Load(), "a refusal is not retried")
	require.Zero(t, hits.Load())
}

func TestWithRetryMaxElapsed(t *testing.T) {
	t.Parallel()

	srv, hits := failingServer(t, 10)
	c := httpclient.New(loopback, httpclient.WithRetryMax(5),
		httpclient.WithRetryWait(time.Second, time.Second), httpclient.WithRetryJitter(0),
		httpclient.WithRetryMaxElapsed(100*time.Millisecond))

	start := time.Now()
	_, err := get(t, c, srv.URL)
	require.Error(t, err)
	require.Equal(t, int32(1), hits.Load(), "a retry past the elapsed bound is not started")
	require.Less(t, time.Since(start), time.Second)
}
