// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for use with [errors.Is]. Each structured error type
// in this package (e.g., [ResponseSizeError], [CircuitBreakerError]) implements
// an Is method that matches the corresponding sentinel, so callers can write:
//
//	if errors.Is(err, client.ErrResponseSizeExceeded) { ... }
var (
	// ErrCircuitBreakerOpen indicates that the circuit breaker is in open state.
	// Returned (as a wrapped [CircuitBreakerError]) by [HTTPClient.Do] and all
	// convenience methods ([HTTPClient.Get], [HTTPClient.Post], etc.) when the
	// per-host or global circuit breaker has tripped. Use [IsCircuitBreakerError]
	// or [IsCircuitBreakerOpen] to inspect the error.
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

	// ErrResponseSizeExceeded indicates response size exceeded the configured limit
	// set via [WithMaxResponseSize]. Returned (as a [ResponseSizeError]) by the
	// circuit-breaker transport when Content-Length exceeds the threshold.
	// Use [IsResponseSizeError] to extract details.
	ErrResponseSizeExceeded = errors.New("response size exceeded limit")

	// ErrMaxRetriesExceeded indicates all retry attempts have been exhausted.
	// Returned (as a [RetryExhaustedError]) by [HTTPClient.Do] and all convenience
	// methods when a retryable failure persists after the configured [WithRetryMax]
	// limit (or the [WithRetryMaxElapsed] budget) is reached. The underlying
	// cause — the last transport error, or the last reply's
	// [UnexpectedStatusError] — is available via [errors.Unwrap] or
	// [IsRetryExhaustedError].
	ErrMaxRetriesExceeded = errors.New("maximum retries exceeded")

	// ErrNonRetryable indicates an error that must not be retried (e.g., context
	// cancellation, certificate errors, SSRF blocks, connection refusals). Returned
	// (as a [NonRetryableError]) by the internal retry policy. The underlying cause
	// is available via [errors.Unwrap].
	ErrNonRetryable = errors.New("non-retryable error")

	// ErrUnexpectedStatus indicates an unexpected HTTP status code was received.
	// Returned (as an [UnexpectedStatusError]) by the retry policy for 4xx/5xx
	// responses that are not retryable. Use [IsUnexpectedStatusError] to extract
	// the status code, host, URI, and method.
	ErrUnexpectedStatus = errors.New("unexpected HTTP status")

	// ErrSSRFBlocked indicates a connection to a private/local IP was blocked by
	// SSRF protection enabled via [WithSSRFProtection]. Returned (as an [SSRFError])
	// at the transport level before TCP connection is established.
	// Use [IsSSRFError] to extract details.
	ErrSSRFBlocked = errors.New("ssrf: connection blocked")

	// ErrSSRFUnsafeTransport identifies a transport configuration that cannot
	// enforce SSRF protection. New panics with an error wrapping this sentinel.
	ErrSSRFUnsafeTransport = errors.New("ssrf: incompatible transport configuration")
)

// DefaultMaxBodySizeInError caps the response body size (in bytes) included in
// [UnexpectedStatusError.Error] output when callers do not override it.
const DefaultMaxBodySizeInError = 1024

// UnexpectedStatusError represents an HTTP response with an unexpected status code.
// It provides detailed information about the failed request including the status code,
// host, URI, and HTTP method used. Matches [ErrUnexpectedStatus] via [errors.Is].
// Use [IsUnexpectedStatusError] to extract the typed error from an error chain.
type UnexpectedStatusError struct {
	// Status contains the HTTP status code returned by the server
	Status int
	// Host is the target host where the request was sent
	Host string
	// URI is the request URI path
	URI string
	// Method is the HTTP method used (GET, POST, etc.)
	Method string
	// Body optionally carries the response body for diagnostics.
	// A truncated preview is included in [UnexpectedStatusError.Error] output.
	Body []byte
}

// Error implements the error interface for UnexpectedStatusError.
func (se UnexpectedStatusError) Error() string {
	msg := fmt.Sprintf("unexpected response status: %d (%s); request: %s %s%s",
		se.Status, http.StatusText(se.Status), se.Method, se.Host, se.URI)
	if len(se.Body) > 0 {
		body := string(se.Body)
		if len(body) > DefaultMaxBodySizeInError {
			body = body[:DefaultMaxBodySizeInError] + "... (truncated)"
		}
		msg += ". Details: " + body
	}
	return msg
}

// Is allows errors.Is to work with UnexpectedStatusError
func (se UnexpectedStatusError) Is(target error) bool {
	return target == ErrUnexpectedStatus
}

// RequestBuilderError represents a configuration error detected during request
// construction (e.g., missing URL). Returned by [RequestBuilder.BuildWithContext]
// and, transitively, by [RequestBuilder.Send].
type RequestBuilderError struct {
	Message string
}

// Error implements the error interface for RequestBuilderError.
func (e *RequestBuilderError) Error() string {
	return e.Message
}

// ResponseSizeError reports that a response exceeded the size limit configured
// via [WithMaxResponseSize]. It matches [ErrResponseSizeExceeded] through [errors.Is].
type ResponseSizeError struct {
	// Limit is the configured maximum response size in bytes.
	Limit int64
	// Size is the actual Content-Length advertised by the server.
	Size int64
}

// Error implements the error interface
func (e *ResponseSizeError) Error() string {
	return fmt.Sprintf("response size %d exceeds limit %d", e.Size, e.Limit)
}

// Is allows errors.Is to work with ResponseSizeError
func (e *ResponseSizeError) Is(target error) bool {
	return target == ErrResponseSizeExceeded
}

// CircuitBreakerError reports that a request was rejected because the circuit
// breaker for the target host is in the open or half-open (too-many-requests)
// state. It matches [ErrCircuitBreakerOpen] through [errors.Is].
type CircuitBreakerError struct {
	// Name identifies the circuit breaker instance (auto-generated or set via [WithBreakerName]).
	Name string
	// State is "open" or "too_many_requests".
	State string
}

// Error implements the error interface
func (e *CircuitBreakerError) Error() string {
	return fmt.Sprintf("circuit breaker %q is %s", e.Name, e.State)
}

// Is allows errors.Is to work with CircuitBreakerError
func (e *CircuitBreakerError) Is(target error) bool {
	return target == ErrCircuitBreakerOpen
}

// RetryExhaustedError reports that all retry attempts have been exhausted.
// It matches [ErrMaxRetriesExceeded] through [errors.Is] and wraps the
// last error encountered via [errors.Unwrap].
type RetryExhaustedError struct {
	// LastError is the error from the final attempt.
	LastError error
	// Attempts is the total number of attempts made (initial + retries).
	Attempts int
	// Method is the HTTP method of the failed request.
	Method string
	// URL is the target URL of the failed request.
	URL string
}

// Error implements the error interface
func (e *RetryExhaustedError) Error() string {
	return fmt.Sprintf("failed after %d attempts for %s %s: %v",
		e.Attempts, e.Method, e.URL, e.LastError)
}

// Is allows errors.Is to work with RetryExhaustedError
func (e *RetryExhaustedError) Is(target error) bool {
	return target == ErrMaxRetriesExceeded
}

// Unwrap returns the underlying error
func (e *RetryExhaustedError) Unwrap() error {
	return e.LastError
}

// NonRetryableError wraps an error that must not be retried by the retry
// layer. The retry policy emits this for conditions like context cancellation,
// certificate errors, SSRF blocks, and connection refusals. It matches
// [ErrNonRetryable] through [errors.Is] and exposes the underlying cause
// via [errors.Unwrap].
type NonRetryableError struct {
	Err error
}

// Error implements the error interface
func (e *NonRetryableError) Error() string {
	return fmt.Sprintf("non-retryable error: %v", e.Err)
}

// Is allows errors.Is to work with NonRetryableError
func (e *NonRetryableError) Is(target error) bool {
	return target == ErrNonRetryable
}

// Unwrap returns the underlying error
func (e *NonRetryableError) Unwrap() error {
	return e.Err
}

// IsResponseSizeError unwraps err looking for a [ResponseSizeError].
// Returns nil when no match is found.
func IsResponseSizeError(err error) *ResponseSizeError {
	target, _ := errors.AsType[*ResponseSizeError](err)
	return target
}

// IsCircuitBreakerError unwraps err looking for a [CircuitBreakerError].
// Returns nil when no match is found.
func IsCircuitBreakerError(err error) *CircuitBreakerError {
	target, _ := errors.AsType[*CircuitBreakerError](err)
	return target
}

// IsRetryExhaustedError unwraps err looking for a [RetryExhaustedError].
// Returns nil when no match is found.
func IsRetryExhaustedError(err error) *RetryExhaustedError {
	target, _ := errors.AsType[*RetryExhaustedError](err)
	return target
}

// SSRFError is returned when a connection to a private/local IP address is blocked
// by SSRF protection enabled via [WithSSRFProtection]. Matches [ErrSSRFBlocked]
// via [errors.Is]. Use [IsSSRFError] to extract the typed error from an error chain.
type SSRFError struct {
	Host string
	IP   string
}

// Error implements the error interface.
func (e *SSRFError) Error() string {
	return fmt.Sprintf("ssrf: connection to private/local address blocked: host=%s ip=%s", e.Host, e.IP)
}

// Is allows errors.Is to work with SSRFError.
func (e *SSRFError) Is(target error) bool {
	return target == ErrSSRFBlocked
}

// IsSSRFError checks if error is due to SSRF protection and returns details.
func IsSSRFError(err error) *SSRFError {
	target, _ := errors.AsType[*SSRFError](err)
	return target
}

// IsTemporaryError reports whether err represents a transient condition that may
// succeed on a subsequent attempt. It checks for [UnexpectedStatusError] with
// status 408, 429, 503, or 504.
func IsTemporaryError(err error) bool {
	if statusErr, ok := errors.AsType[*UnexpectedStatusError](err); ok {
		switch statusErr.Status {
		case http.StatusTooManyRequests,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
			http.StatusRequestTimeout:
			return true
		}
	}

	return false
}
