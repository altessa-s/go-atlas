// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestUnexpectedStatusError_Error(t *testing.T) {
	tests := []struct {
		name   string
		err    UnexpectedStatusError
		expect string
	}{
		{"not_found", UnexpectedStatusError{Status: 404, Method: "GET", Host: "example.com", URI: "/users"}, "unexpected response status: 404 (Not Found); request: GET example.com/users"},
		{"server_error", UnexpectedStatusError{Status: 500, Method: "POST", Host: "api.test", URI: "/data"}, "unexpected response status: 500 (Internal Server Error); request: POST api.test/data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expect {
				t.Fatalf("Error() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestUnexpectedStatusError_Is(t *testing.T) {
	err := UnexpectedStatusError{Status: 404}
	if !errors.Is(err, ErrUnexpectedStatus) {
		t.Fatal("should match ErrUnexpectedStatus")
	}
	if errors.Is(err, ErrCircuitBreakerOpen) {
		t.Fatal("should not match ErrCircuitBreakerOpen")
	}
}

func TestRequestBuilderError_Error(t *testing.T) {
	err := &RequestBuilderError{Message: "bad request"}
	if err.Error() != "bad request" {
		t.Fatalf("Error() = %q", err.Error())
	}
}

func TestResponseSizeError(t *testing.T) {
	err := &ResponseSizeError{Limit: 1000, Size: 2000}
	if err.Error() != "response size 2000 exceeds limit 1000" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, ErrResponseSizeExceeded) {
		t.Fatal("should match ErrResponseSizeExceeded")
	}
}

func TestCircuitBreakerError(t *testing.T) {
	err := &CircuitBreakerError{Name: "test-cb", State: "open"}
	if err.Error() != `circuit breaker "test-cb" is open` {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, ErrCircuitBreakerOpen) {
		t.Fatal("should match ErrCircuitBreakerOpen")
	}
}

func TestRateLimitError(t *testing.T) {
	t.Run("with_retry_after", func(t *testing.T) {
		err := &RateLimitError{Host: "api.test", RetryAfter: 5 * time.Second}
		got := err.Error()
		if got != `rate limit exceeded for host "api.test", retry after 5s` {
			t.Fatalf("Error() = %q", got)
		}
	})
	t.Run("without_retry_after", func(t *testing.T) {
		err := &RateLimitError{Host: "api.test"}
		got := err.Error()
		if got != `rate limit exceeded for host "api.test"` {
			t.Fatalf("Error() = %q", got)
		}
	})
	t.Run("is", func(t *testing.T) {
		err := &RateLimitError{Host: "test"}
		if !errors.Is(err, ErrRateLimited) {
			t.Fatal("should match ErrRateLimited")
		}
	})
}

func TestRetryExhaustedError(t *testing.T) {
	inner := errors.New("connection refused")
	err := &RetryExhaustedError{LastError: inner, Attempts: 3, Method: "GET", URL: "http://test"}
	if err.Error() != "failed after 3 attempts for GET http://test: connection refused" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Fatal("should match ErrMaxRetriesExceeded")
	}
	if err.Unwrap() != inner {
		t.Fatal("Unwrap() should return inner error")
	}
}

func TestNonRetryableError(t *testing.T) {
	inner := errors.New("bad cert")
	err := &NonRetryableError{Err: inner}
	if err.Error() != "non-retryable error: bad cert" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, ErrNonRetryable) {
		t.Fatal("should match ErrNonRetryable")
	}
	if err.Unwrap() != inner {
		t.Fatal("Unwrap() should return inner error")
	}
}

func TestIsResponseSizeError(t *testing.T) {
	sizeErr := &ResponseSizeError{Limit: 100, Size: 200}
	if IsResponseSizeError(sizeErr) == nil {
		t.Fatal("should return error")
	}
	if IsResponseSizeError(errors.New("other")) != nil {
		t.Fatal("should return nil")
	}
}

func TestIsCircuitBreakerError(t *testing.T) {
	cbErr := &CircuitBreakerError{Name: "test"}
	if IsCircuitBreakerError(cbErr) == nil {
		t.Fatal("should return error")
	}
	if IsCircuitBreakerError(errors.New("other")) != nil {
		t.Fatal("should return nil")
	}
}

func TestIsRateLimitError(t *testing.T) {
	rlErr := &RateLimitError{Host: "test"}
	if IsRateLimitError(rlErr) == nil {
		t.Fatal("should return error")
	}
	if IsRateLimitError(errors.New("other")) != nil {
		t.Fatal("should return nil")
	}
}

func TestIsRetryExhaustedError(t *testing.T) {
	reErr := &RetryExhaustedError{Attempts: 3}
	if IsRetryExhaustedError(reErr) == nil {
		t.Fatal("should return error")
	}
	if IsRetryExhaustedError(errors.New("other")) != nil {
		t.Fatal("should return nil")
	}
}

func TestIsTemporaryError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"too_many_requests", &UnexpectedStatusError{Status: http.StatusTooManyRequests}, true},
		{"service_unavailable", &UnexpectedStatusError{Status: http.StatusServiceUnavailable}, true},
		{"gateway_timeout", &UnexpectedStatusError{Status: http.StatusGatewayTimeout}, true},
		{"request_timeout", &UnexpectedStatusError{Status: http.StatusRequestTimeout}, true},
		{"not_found", &UnexpectedStatusError{Status: http.StatusNotFound}, false},
		{"plain_error", errors.New("plain"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTemporaryError(tt.err); got != tt.want {
				t.Fatalf("IsTemporaryError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsCheckers_WrappedErrors(t *testing.T) {
	sizeErr := &ResponseSizeError{Limit: 100, Size: 200}
	wrapped := fmt.Errorf("wrapped: %w", sizeErr)
	if IsResponseSizeError(wrapped) == nil {
		t.Fatal("should unwrap to find ResponseSizeError")
	}
}

func TestSentinelErrors_NotNil(t *testing.T) {
	sentinels := []error{
		ErrCircuitBreakerOpen,
		ErrResponseSizeExceeded,
		ErrRateLimited,
		ErrMaxRetriesExceeded,
		ErrNonRetryable,
		ErrUnexpectedStatus,
	}
	for _, err := range sentinels {
		if err == nil {
			t.Fatal("sentinel error is nil")
		}
		if err.Error() == "" {
			t.Fatalf("sentinel error has empty message: %v", err)
		}
	}
}
