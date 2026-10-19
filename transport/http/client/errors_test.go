// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnexpectedStatusError_Error(t *testing.T) {
	tests := []struct {
		name   string
		err    UnexpectedStatusError
		expect string
	}{
		{
			"not_found",
			UnexpectedStatusError{Status: 404, Method: "GET", Host: "example.com", URI: "/users"},
			"unexpected response status: 404 (Not Found); request: GET example.com/users",
		},
		{
			"server_error",
			UnexpectedStatusError{Status: 500, Method: "POST", Host: "api.test", URI: "/data"},
			"unexpected response status: 500 (Internal Server Error); request: POST api.test/data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			require.Equal(t, tt.expect, got)
		})
	}
}

func TestUnexpectedStatusError_Is(t *testing.T) {
	err := UnexpectedStatusError{Status: 404}
	require.True(t, errors.Is(err, ErrUnexpectedStatus), "should match ErrUnexpectedStatus")
	require.False(t, errors.Is(err, ErrCircuitBreakerOpen), "should not match ErrCircuitBreakerOpen")
}

func TestRequestBuilderError_Error(t *testing.T) {
	err := &RequestBuilderError{Message: "bad request"}
	require.Equal(t, "bad request", err.Error())
}

func TestResponseSizeError(t *testing.T) {
	err := &ResponseSizeError{Limit: 1000, Size: 2000}
	require.Equal(t, "response size 2000 exceeds limit 1000", err.Error())
	require.True(t, errors.Is(err, ErrResponseSizeExceeded), "should match ErrResponseSizeExceeded")
}

func TestCircuitBreakerError(t *testing.T) {
	err := &CircuitBreakerError{Name: "test-cb", State: "open"}
	require.Equal(t, `circuit breaker "test-cb" is open`, err.Error())
	require.True(t, errors.Is(err, ErrCircuitBreakerOpen), "should match ErrCircuitBreakerOpen")
}

func TestRetryExhaustedError(t *testing.T) {
	inner := errors.New("connection refused")
	err := &RetryExhaustedError{LastError: inner, Attempts: 3, Method: "GET", URL: "http://test"}
	require.Equal(t, "failed after 3 attempts for GET http://test: connection refused", err.Error())
	require.True(t, errors.Is(err, ErrMaxRetriesExceeded), "should match ErrMaxRetriesExceeded")
	require.Equal(t, inner, err.Unwrap())
}

func TestNonRetryableError(t *testing.T) {
	inner := errors.New("bad cert")
	err := &NonRetryableError{Err: inner}
	require.Equal(t, "non-retryable error: bad cert", err.Error())
	require.True(t, errors.Is(err, ErrNonRetryable), "should match ErrNonRetryable")
	require.Equal(t, inner, err.Unwrap())
}

func TestIsResponseSizeError(t *testing.T) {
	sizeErr := &ResponseSizeError{Limit: 100, Size: 200}
	require.NotNil(t, IsResponseSizeError(sizeErr))
	require.Nil(t, IsResponseSizeError(errors.New("other")))
}

func TestIsCircuitBreakerError(t *testing.T) {
	cbErr := &CircuitBreakerError{Name: "test"}
	require.NotNil(t, IsCircuitBreakerError(cbErr))
	require.Nil(t, IsCircuitBreakerError(errors.New("other")))
}

func TestIsRetryExhaustedError(t *testing.T) {
	reErr := &RetryExhaustedError{Attempts: 3}
	require.NotNil(t, IsRetryExhaustedError(reErr))
	require.Nil(t, IsRetryExhaustedError(errors.New("other")))
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
			got := IsTemporaryError(tt.err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsCheckers_WrappedErrors(t *testing.T) {
	sizeErr := &ResponseSizeError{Limit: 100, Size: 200}
	wrapped := fmt.Errorf("wrapped: %w", sizeErr)
	require.NotNil(t, IsResponseSizeError(wrapped))
}
