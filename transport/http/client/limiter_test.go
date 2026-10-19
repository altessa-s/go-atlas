// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/http/client"
	"github.com/altessa-s/go-atlas/transport/http/client/limiters"
)

// limiterFunc adapts a function to limiters.RequestsLimiter.
type limiterFunc func(ctx context.Context, key string) error

func (f limiterFunc) Allow(ctx context.Context, key string) error { return f(ctx, key) }

func TestNew_LimiterDenialIsRateLimitError(t *testing.T) {
	t.Parallel()

	errQuota := errors.New("quota exhausted")
	custom := &limiters.RateLimitError{Host: "preset", RetryAfter: time.Second}

	tests := []struct {
		name      string
		allow     func(ctx context.Context, key string) error
		wantCause error
		wantHost  string
		wantAfter time.Duration
	}{
		{
			name:      "limiter error is wrapped",
			allow:     func(context.Context, string) error { return errQuota },
			wantCause: errQuota,
			wantHost:  "127.0.0.1",
		},
		{
			name:      "context error stays visible",
			allow:     func(context.Context, string) error { return context.DeadlineExceeded },
			wantCause: context.DeadlineExceeded,
			wantHost:  "127.0.0.1",
		},
		{
			name:      "limiter RateLimitError passes through",
			allow:     func(context.Context, string) error { return custom },
			wantHost:  "preset",
			wantAfter: time.Second,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(srv.Close)

			c := client.New(client.WithoutSSRFProtection(), client.WithLimiter(limiterFunc(tc.allow)))
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
			require.NoError(t, err)

			resp, err := c.Do(req)
			if resp != nil {
				require.NoError(t, resp.Body.Close())
			}
			require.Error(t, err)
			require.ErrorIs(t, err, limiters.ErrRateLimited)

			rl := limiters.IsRateLimitError(err)
			require.NotNil(t, rl)
			require.Equal(t, tc.wantHost, rl.Host)
			require.Equal(t, tc.wantAfter, rl.RetryAfter)
			if tc.wantCause != nil {
				require.ErrorIs(t, err, tc.wantCause)
			}

			_, isURLErr := errors.AsType[*url.Error](err)
			require.True(t, isURLErr, "net/http wraps transport errors in *url.Error")
			require.Zero(t, hits.Load(), "a denied request must not reach the server")
		})
	}
}

func TestNew_LimiterAllowReachesServer(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	c := client.New(client.WithoutSSRFProtection(),
		client.WithLimiter(limiterFunc(func(context.Context, string) error { return nil })))
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := c.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}
