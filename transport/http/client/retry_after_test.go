// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
)

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		value  string
		max    time.Duration
		want   time.Duration
		wantOK bool
	}{
		{"absent", "", 0, 0, false},
		{"seconds", "120", 0, 2 * time.Minute, true},
		{"zero seconds", "0", 0, 0, true},
		{"spaces", " 5 ", 0, 5 * time.Second, true},
		{"capped seconds", "3600", time.Minute, time.Minute, true},
		{"huge seconds", "99999999999999999999", time.Minute, time.Minute, true},
		{"negative", "-5", 0, 0, false},
		{"signed", "+5", 0, 0, false},
		{"fraction", "1.5", 0, 0, false},
		{"garbage", "soon", 0, 0, false},
		{"date ahead", now.Add(90 * time.Second).Format(http.TimeFormat), 0, 90 * time.Second, true},
		{"date ahead capped", now.Add(time.Hour).Format(http.TimeFormat), time.Minute, time.Minute, true},
		{"date past", now.Add(-time.Hour).Format(http.TimeFormat), 0, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := http.Header{}
			if tc.value != "" {
				h.Set("Retry-After", tc.value)
			}
			got, ok := httpclient.ParseRetryAfter(h, now, tc.max)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

// retryAfterServer answers the first request with status and Retry-After, and
// every later one with 200.
func retryAfterServer(t *testing.T, status int, retryAfter string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", retryAfter)
			w.WriteHeader(status)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// timedGet sends one GET through c and returns how long it took.
func timedGet(t *testing.T, c *http.Client, url string) time.Duration {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	start := time.Now()
	resp, err := c.Do(req)
	elapsed := time.Since(start)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return elapsed
}

func TestRetryHonorsRetryAfter(t *testing.T) {
	t.Parallel()

	const capDelay = 150 * time.Millisecond
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			srv, hits := retryAfterServer(t, status, "3600")
			c := httpclient.New(loopback, httpclient.WithRetryMax(1),
				httpclient.WithRetryWait(time.Millisecond, time.Millisecond),
				httpclient.WithRetryAfterMax(capDelay))

			elapsed := timedGet(t, c, srv.URL)
			require.Equal(t, int32(2), hits.Load())
			require.GreaterOrEqual(t, elapsed, capDelay, "the retry waits for Retry-After, capped")
			require.Less(t, elapsed, 10*capDelay)
		})
	}
}

func TestRetryAfterIgnored(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		opts   []httpclient.Option
	}{
		{"disabled", http.StatusTooManyRequests, []httpclient.Option{httpclient.WithRetryAfterMax(0)}},
		{"other status", http.StatusBadGateway, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv, hits := retryAfterServer(t, tc.status, "3600")
			opts := append([]httpclient.Option{loopback, httpclient.WithRetryMax(1),
				httpclient.WithRetryWait(time.Millisecond, time.Millisecond)}, tc.opts...)
			elapsed := timedGet(t, httpclient.New(opts...), srv.URL)
			require.Equal(t, int32(2), hits.Load())
			require.Less(t, elapsed, 5*time.Second, "Retry-After does not delay the retry")
		})
	}
}

func TestWithRetryAfterStatuses(t *testing.T) {
	t.Parallel()

	const capDelay = 150 * time.Millisecond
	statuses := []int{http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusBadGateway, http.StatusGatewayTimeout}
	for _, status := range statuses {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			srv, hits := retryAfterServer(t, status, "3600")
			c := httpclient.New(loopback, httpclient.WithRetryMax(1),
				httpclient.WithRetryWait(time.Millisecond, time.Millisecond),
				httpclient.WithRetryAfterMax(capDelay),
				httpclient.WithRetryAfterStatuses(statuses...),
				// The default classification retries only some of these.
				httpclient.WithRetryPolicyHandler(func(_ context.Context, resp *http.Response, err error) (bool, error) {
					return err != nil || resp.StatusCode >= http.StatusBadRequest, nil
				}))

			elapsed := timedGet(t, c, srv.URL)
			require.Equal(t, int32(2), hits.Load())
			require.GreaterOrEqual(t, elapsed, capDelay, "the configured status honors Retry-After")
		})
	}

	t.Run("replaces the default set", func(t *testing.T) {
		t.Parallel()

		srv, hits := retryAfterServer(t, http.StatusServiceUnavailable, "3600")
		c := httpclient.New(loopback, httpclient.WithRetryMax(1),
			httpclient.WithRetryWait(time.Millisecond, time.Millisecond),
			httpclient.WithRetryAfterStatuses(http.StatusBadGateway))
		elapsed := timedGet(t, c, srv.URL)
		require.Equal(t, int32(2), hits.Load())
		require.Less(t, elapsed, 5*time.Second, "503 is no longer in the set")
	})
}
