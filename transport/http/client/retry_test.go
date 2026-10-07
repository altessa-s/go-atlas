// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreretry "github.com/altessa-s/go-atlas/core/retry"
)

func newTestRetryOpts(maxAttempts int) []coreretry.Option {
	return []coreretry.Option{
		coreretry.WithMaxAttempts(maxAttempts),
		coreretry.WithNextDelay(coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: time.Nanosecond,
			Factor:    1,
		})),
	}
}

func TestRetryRoundTripper_Success(t *testing.T) {
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		}),
		retryOpts:   newTestRetryOpts(3),
		maxAttempts: 3,
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRetryRoundTripper_RetryThenSuccess(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			n := calls.Add(1)
			if n < 3 {
				return nil, errors.New("transient")
			}
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		}),
		retryOpts:   newTestRetryOpts(5),
		maxAttempts: 5,
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(3), calls.Load())
}

func TestRetryRoundTripper_Exhaustion(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("always fail")
		}),
		retryOpts:   newTestRetryOpts(2), // 3 total attempts (0, 1, 2)
		maxAttempts: 2,
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	require.Error(t, err)
	require.Equal(t, int32(3), calls.Load())
}

func TestRetryRoundTripper_NonRetryableStops(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, &NonRetryableError{Err: errors.New("cert error")}
		}),
		retryOpts:   newTestRetryOpts(5),
		maxAttempts: 5,
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	require.Error(t, err)
	got := calls.Load()
	require.Equal(t, int32(1), got)
}

func TestRetryRoundTripper_BodyReplay(t *testing.T) {
	var bodies []string
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			b, _ := io.ReadAll(req.Body)
			bodies = append(bodies, string(b))
			if len(bodies) < 3 {
				return nil, errors.New("transient")
			}
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		}),
		retryOpts:   newTestRetryOpts(5),
		maxAttempts: 5,
	}

	req, _ := http.NewRequestWithContext(t.Context(), "POST", "http://example.com", strings.NewReader("payload"))
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	for _, b := range bodies {
		require.Equal(t, "payload", b)
	}
}

func TestRetryRoundTripper_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			if calls.Add(1) >= 1 {
				cancel()
			}
			return nil, errors.New("transient")
		}),
		retryOpts:   newTestRetryOpts(10),
		maxAttempts: 10,
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestRetryRoundTripper_RetryableStatus(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			n := calls.Add(1)
			if n < 3 {
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Body:       http.NoBody,
					Request:    req,
				}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
		}),
		retryOpts:   newTestRetryOpts(5),
		maxAttempts: 5,
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com/path", nil)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRetryRoundTripper_UnexpectedStatus(t *testing.T) {
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}),
		retryOpts:   newTestRetryOpts(3),
		maxAttempts: 3,
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com/path", nil)
	resp, err := rt.RoundTrip(req) //nolint:bodyclose // asserted nil
	require.Error(t, err)
	require.Nil(t, resp, "a response must never accompany an error")
	statusErr, ok := errors.AsType[*UnexpectedStatusError](err)
	require.True(t, ok)
	require.Equal(t, http.StatusForbidden, statusErr.Status)
}

func TestRetryRoundTripper_ErrorHandler(t *testing.T) {
	handlerCalled := false
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("always fail")
		}),
		retryOpts:   newTestRetryOpts(1),
		maxAttempts: 1,
		errorHandler: func(resp *http.Response, err error, numTries int) (*http.Response, error) {
			handlerCalled = true
			return resp, err
		},
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	_, _ = rt.RoundTrip(req)
	require.True(t, handlerCalled, "ErrorHandler was not called")
}

func TestRetryRoundTripper_RetryPolicyHandler(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
		}),
		retryOpts:   newTestRetryOpts(5),
		maxAttempts: 5,
		retryPolicyHandler: func(_ context.Context, resp *http.Response, err error) (bool, error) {
			// Force retry on first call
			if calls.Load() < 3 {
				return true, nil
			}
			return false, nil
		},
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(3), calls.Load())
}

func TestRetryRoundTripper_ZeroRetries(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
		}),
		retryOpts:   nil, // single attempt, no retries
		maxAttempts: 0,
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, int32(1), calls.Load())
}

func TestRetryRoundTripper_OneRetryReplaysStreamingBody(t *testing.T) {
	t.Parallel()

	var bodies []string
	rt := &retryRoundTripper{
		next: testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
			b, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.NoError(t, req.Body.Close())
			bodies = append(bodies, string(b))
			status := http.StatusOK
			if len(bodies) == 1 {
				status = http.StatusServiceUnavailable
			}
			return &http.Response{StatusCode: status, Body: http.NoBody, Request: req}, nil
		}),
		retryOpts:   newTestRetryOpts(1), // one retry: two attempts
		maxAttempts: 1,
	}

	// A plain io.ReadCloser has no GetBody, so only buffering can replay it.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://example.com",
		io.NopCloser(strings.NewReader("payload")))
	require.NoError(t, err)
	require.Nil(t, req.GetBody)

	resp, err := rt.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []string{"payload", "payload"}, bodies)
}

// trackedBody is a response body that records whether it was closed.
type trackedBody struct {
	io.Reader
	closed atomic.Bool
}

func newTrackedBody() *trackedBody { return &trackedBody{Reader: strings.NewReader("body")} }

func (b *trackedBody) Close() error {
	b.closed.Store(true)
	return nil
}

func TestClient_ErrorResponsesAreClosed(t *testing.T) {
	t.Parallel()

	errHandled := errors.New("handled")
	tests := []struct {
		name         string
		status       int
		retryMax     int
		handler      func(bodies *[]*trackedBody) ErrorHandler
		wantAttempts int
		wantErr      bool
		wantOpenLast bool // the last body is returned to the caller, open
	}{
		{name: "non-retryable 403", status: http.StatusForbidden, retryMax: 2, wantAttempts: 1, wantErr: true},
		{name: "exhausted 503", status: http.StatusServiceUnavailable, retryMax: 2, wantAttempts: 3, wantErr: true},
		{
			name: "handler drops response", status: http.StatusForbidden, wantAttempts: 1, wantErr: true,
			handler: func(*[]*trackedBody) ErrorHandler {
				return func(_ *http.Response, _ error, _ int) (*http.Response, error) { return nil, errHandled }
			},
		},
		{
			name: "handler replaces response with error", status: http.StatusForbidden, wantAttempts: 1, wantErr: true,
			handler: func(bodies *[]*trackedBody) ErrorHandler {
				return func(_ *http.Response, _ error, _ int) (*http.Response, error) {
					b := newTrackedBody()
					*bodies = append(*bodies, b)
					return &http.Response{StatusCode: http.StatusTeapot, Body: b}, errHandled
				}
			},
		},
		{
			name: "handler passes response through", status: http.StatusForbidden, wantAttempts: 1, wantOpenLast: true,
			handler: func(*[]*trackedBody) ErrorHandler {
				return func(resp *http.Response, _ error, _ int) (*http.Response, error) { return resp, nil }
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var bodies []*trackedBody
			var attempts int
			transport := testhelpers.RoundTripFunc(func(req *http.Request) (*http.Response, error) {
				attempts++
				b := newTrackedBody()
				bodies = append(bodies, b)
				return &http.Response{StatusCode: tc.status, Body: b, Request: req}, nil
			})
			opts := []Option{
				WithClient(&http.Client{Transport: transport}),
				WithRetryMax(tc.retryMax),
				WithRetryWait(time.Nanosecond, time.Nanosecond),
			}
			if tc.handler != nil {
				opts = append(opts, WithErrorHandler(tc.handler(&bodies)))
			}
			c := New(opts...)

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/x", nil)
			require.NoError(t, err)
			resp, err := c.Do(req)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
			}
			require.Equal(t, tc.wantAttempts, attempts)

			for i, b := range bodies {
				if tc.wantOpenLast && i == len(bodies)-1 {
					require.False(t, b.closed.Load(), "returned body must stay open")
					require.NoError(t, resp.Body.Close())
					continue
				}
				require.True(t, b.closed.Load(), "body %d leaked", i)
			}
		})
	}
}

func TestClient_CircuitBreakerOpenDetected(t *testing.T) {
	t.Parallel()

	transport := testhelpers.RoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset")
	})
	c := New(
		WithClient(&http.Client{Transport: transport}),
		WithRetryMax(0),
		WithCircuitBreakerSettings("breaker.test", &CircuitBreakerSettings{
			ReadyToTrip: func(gobreaker.Counts) bool { return true },
			Timeout:     time.Hour,
		}),
	)

	do := func() error {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://breaker.test/", nil)
		require.NoError(t, err)
		resp, err := c.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		return err
	}

	require.Error(t, do()) // trips the breaker
	err := do()
	require.Error(t, err)
	require.True(t, IsCircuitBreakerOpen(err), "client error must be detected as open breaker: %v", err)
}
