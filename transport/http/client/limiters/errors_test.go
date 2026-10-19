// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/transport/http/client/limiters"
)

type limiterFunc func(ctx context.Context, key string) error

func (f limiterFunc) Allow(ctx context.Context, key string) error { return f(ctx, key) }

func TestRateLimitError(t *testing.T) {
	t.Parallel()

	cause := errors.New("quota exhausted")
	tests := []struct {
		name string
		err  *limiters.RateLimitError
		want string
	}{
		{"host only", &limiters.RateLimitError{Host: "api.test"}, `rate limit exceeded for host "api.test"`},
		{"retry after", &limiters.RateLimitError{Host: "api.test", RetryAfter: 5 * time.Second},
			`rate limit exceeded for host "api.test", retry after 5s`},
		{"cause", &limiters.RateLimitError{Host: "api.test", Err: cause}, `rate limit exceeded for host "api.test": quota exhausted`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.err.Error())
			require.ErrorIs(t, tc.err, limiters.ErrRateLimited)
			require.Same(t, tc.err, limiters.IsRateLimitError(tc.err))
		})
	}

	withCause := &limiters.RateLimitError{Host: "api.test", Err: cause}
	require.ErrorIs(t, withCause, cause)
	require.Equal(t, cause, withCause.Unwrap())
	require.Nil(t, limiters.IsRateLimitError(errors.New("other")))
}

// TestRoundTripper_DenialIsRateLimitError uses the round tripper directly,
// without the client package: a denial is a *RateLimitError for the request
// host wrapping the limiter's error, and the request never reaches next.
func TestRoundTripper_DenialIsRateLimitError(t *testing.T) {
	t.Parallel()

	errQuota := errors.New("quota exhausted")
	preset := &limiters.RateLimitError{Host: "preset", RetryAfter: time.Second}
	tests := []struct {
		name      string
		limiter   limiterFunc
		wantCause error
		wantHost  string
		wantAfter time.Duration
	}{
		{"limiter error", func(context.Context, string) error { return errQuota }, errQuota, "api.test", 0},
		{"context error", func(context.Context, string) error { return context.Canceled }, context.Canceled, "api.test", 0},
		{"preset RateLimitError", func(context.Context, string) error { return preset }, nil, "preset", time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var hits atomic.Int32
			next := testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
				hits.Add(1)
				return &http.Response{StatusCode: http.StatusOK}, nil
			})
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.test/path?token=secret", nil)
			require.NoError(t, err)

			resp, err := limiters.NewRoundTripper(next, tc.limiter).RoundTrip(req)
			require.Nil(t, resp)
			require.ErrorIs(t, err, limiters.ErrRateLimited)
			rl := limiters.IsRateLimitError(err)
			require.NotNil(t, rl)
			require.Equal(t, tc.wantHost, rl.Host)
			require.Equal(t, tc.wantAfter, rl.RetryAfter)
			if tc.wantCause != nil {
				require.ErrorIs(t, err, tc.wantCause)
			}
			require.NotContains(t, err.Error(), "secret", "the error must not quote the request URL")
			require.Zero(t, hits.Load())
		})
	}
}
