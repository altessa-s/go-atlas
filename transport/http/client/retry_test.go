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

	coreretry "github.com/altessa-s/go-atlas/core/runtime/retry"
)

// roundTripFunc is a test helper that implements http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newTestRetryCfg(maxAttempts int) coreretry.Config {
	return coreretry.Config{
		MaxAttempts: maxAttempts,
		NextDelay: coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: time.Nanosecond,
			Factor:    1,
		}),
	}
}

func TestRetryRoundTripper_Success(t *testing.T) {
	rt := &retryRoundTripper{
		next: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		}),
		cfg: newTestRetryCfg(3),
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestRetryRoundTripper_RetryThenSuccess(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			n := calls.Add(1)
			if n < 3 {
				return nil, errors.New("transient")
			}
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		}),
		cfg: newTestRetryCfg(5),
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}
}

func TestRetryRoundTripper_Exhaustion(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, errors.New("always fail")
		}),
		cfg: newTestRetryCfg(2), // 3 total attempts (0, 1, 2)
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}
}

func TestRetryRoundTripper_NonRetryableStops(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, &NonRetryableError{Err: errors.New("cert error")}
		}),
		cfg: newTestRetryCfg(5),
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1 (should not retry)", got)
	}
}

func TestRetryRoundTripper_BodyReplay(t *testing.T) {
	var bodies []string
	rt := &retryRoundTripper{
		next: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			b, _ := io.ReadAll(req.Body)
			bodies = append(bodies, string(b))
			if len(bodies) < 3 {
				return nil, errors.New("transient")
			}
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
		}),
		cfg: newTestRetryCfg(5),
	}

	req, _ := http.NewRequestWithContext(t.Context(), "POST", "http://example.com", strings.NewReader("payload"))
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	for i, b := range bodies {
		if b != "payload" {
			t.Errorf("attempt %d body = %q, want %q", i, b, "payload")
		}
	}
}

func TestRetryRoundTripper_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			if calls.Add(1) >= 1 {
				cancel()
			}
			return nil, errors.New("transient")
		}),
		cfg: newTestRetryCfg(10),
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	_, err := rt.RoundTrip(req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestRetryRoundTripper_RetryableStatus(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: roundTripFunc(func(req *http.Request) (*http.Response, error) {
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
		cfg: newTestRetryCfg(5),
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com/path", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestRetryRoundTripper_UnexpectedStatus(t *testing.T) {
	rt := &retryRoundTripper{
		next: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Body:       http.NoBody,
				Request:    req,
			}, nil
		}),
		cfg: newTestRetryCfg(3),
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com/path", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	var statusErr *UnexpectedStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected UnexpectedStatusError, got %T: %v", err, err)
	}
	if statusErr.Status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", statusErr.Status)
	}
}

func TestRetryRoundTripper_ErrorHandler(t *testing.T) {
	handlerCalled := false
	rt := &retryRoundTripper{
		next: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("always fail")
		}),
		cfg: newTestRetryCfg(1),
		errorHandler: func(resp *http.Response, err error, numTries int) (*http.Response, error) {
			handlerCalled = true
			return resp, err
		},
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	_, _ = rt.RoundTrip(req)
	if !handlerCalled {
		t.Fatal("ErrorHandler was not called")
	}
}

func TestRetryRoundTripper_RetryPolicyHandler(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
		}),
		cfg: newTestRetryCfg(5),
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}
}

func TestRetryRoundTripper_ZeroRetries(t *testing.T) {
	var calls atomic.Int32
	rt := &retryRoundTripper{
		next: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
		}),
		cfg: coreretry.Config{MaxAttempts: 0}, // single attempt, no retries
	}

	req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}
