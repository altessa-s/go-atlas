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

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreerrors "github.com/altessa-s/go-atlas/core/errors"
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
	_, err := rt.RoundTrip(req)
	require.Error(t, err)
	statusErr, ok := coreerrors.AsType[*UnexpectedStatusError](err)
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
