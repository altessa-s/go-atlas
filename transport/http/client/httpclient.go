// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters"

	coreretry "github.com/altessa-s/go-atlas/core/retry"
)

const (
	// DefaultRetryWaitMin defines the minimum wait time between retry attempts
	DefaultRetryWaitMin = 500 * time.Millisecond
	// DefaultRetryWaitMax defines the maximum wait time between retry attempts
	DefaultRetryWaitMax = 10 * time.Second
	// DefaultRetryMax defines the default maximum number of retry attempts
	DefaultRetryMax = 5
	// DefaultRetryFactor is the exponential backoff multiplier for retry delays.
	DefaultRetryFactor = 1.5
	// DefaultRetryJitter is the fraction of jitter added to retry delays.
	DefaultRetryJitter = 0.25
)

// ErrorHandler defines the callback for intercepting and handling errors during the retry process.
// Set it via [WithErrorHandler]. The handler receives the HTTP response (may be nil), the error
// that occurred, and the number of retry attempts made. It can modify or wrap the error before
// returning, allowing for custom error handling logic. When the retries are exhausted the error
// is a [RetryExhaustedError].
//
// Body ownership: when the handler returns a response other than the one it received, the
// received response's body is closed. When it returns a non-nil error, any returned response
// body is closed and the caller receives only the error, as [http.RoundTripper] requires.
type ErrorHandler func(resp *http.Response, err error, numTries int) (*http.Response, error)

// RetryPolicyHandler defines the callback that determines whether a request should be retried.
// Set it via [WithRetryPolicyHandler]. The handler receives the request context, HTTP response
// (may be nil), and any error that occurred. It returns true if the request should be retried,
// false otherwise. The handler can also return a modified error to provide additional context.
type RetryPolicyHandler func(ctx context.Context, resp *http.Response, err error) (bool, error)

// Client holds the resolved configuration and produces a fully-wired
// [net/http.Client] with retry, circuit breaker, rate limiting, and SSRF
// protection layers. It is used internally by [New]; most callers should
// use [New] or [NewHTTPClient] directly.
type Client struct {
	options options
}

// New creates a new *[net/http.Client] with automatic retries, exponential backoff,
// and circuit breaker functionality. The client is configured using functional [Option]
// values and returns a standard *[net/http.Client] that can be used with any code
// expecting that type. For a higher-level API with convenience methods, use [NewHTTPClient].
// If no options are provided, sensible defaults are used ([DefaultRetryMax],
// [DefaultRetryWaitMin], [DefaultRetryWaitMax]).
// New panics with an error wrapping [ErrSSRFUnsafeTransport] when default SSRF
// protection is combined with a proxy TLS configuration, a custom TLS dialer, a
// proxy resolver set on a caller transport or a non-standard transport. Use
// [WithoutSSRFProtection] for trusted configurations that need them. Explicit
// proxy options are allowed with SSRF protection (see [ProxyFunc]).
//
// Example:
//
//	client := httpclient.New(
//	    httpclient.WithRetryMax(3),
//	    httpclient.WithRetryWait(1*time.Second, 5*time.Second),
//	    httpclient.WithLogger(slog.Default()),
//	)
//
// Example with default configuration:
//
//	client := httpclient.New()
//	resp, err := client.Get("https://api.example.com/users")
func New(opt ...Option) *http.Client {
	return (&Client{options: *newOptions(opt...)}).retractableClient()
}

func (c *Client) retractableClient() *http.Client {
	if err := validateSSRFTransport(&c.options); err != nil {
		panic(err)
	}
	m := newHTTPClientMetrics(c.options.collector, c.options.metricsSubsystem)
	cbClient := newCircuitBreakerClient(c.options, m)

	hc := newHTTPClientHealth(c.options, cbClient)
	cbClient.health.Store(hc)
	hc.register()

	stdClient := cbClient.standardClient()

	// The retry layer owns cookie handling: the jar of WithCookieJar, or else
	// the one of the WithClient client, is used per attempt and per redirect
	// hop, and the returned client gets none, so nothing is sent or stored
	// twice.
	jar := c.options.cookieJar
	if jar == nil {
		jar = stdClient.Jar
	}
	stdClient.Jar = nil
	if jar != nil {
		stdClient.CheckRedirect = redirectPolicy(stdClient.CheckRedirect)
	}

	retryMax, retryPolicy := c.options.retryMax, c.options.retryPolicyHandler
	if c.options.withoutRetry {
		retryMax, retryPolicy = 0, passThroughPolicy
	}

	retryOpts := []coreretry.Option{
		coreretry.WithMaxAttempts(retryMax),
		coreretry.WithMaxElapsedTime(c.options.retryMaxElapsed),
		coreretry.WithNextDelay(retryAfterDelay(coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: c.options.retryWaitMin,
			MaxDelay:  c.options.retryWaitMax,
			Factor:    c.options.retryFactor,
			Jitter:    c.options.retryJitter,
		}))),
	}

	// With WithLimiterPerAttempt the limiter sits under the retry layer, so
	// every attempt passes it; otherwise it wraps the whole retry loop.
	next := stdClient.Transport
	perAttempt := c.options.limiter != nil && c.options.limiterPerAttempt
	if perAttempt {
		next = limiters.NewRoundTripper(next, c.options.limiter)
	}

	rt := &retryRoundTripper{
		next:               next,
		retryOpts:          retryOpts,
		maxAttempts:        retryMax,
		logger:             c.options.logger,
		errorHandler:       c.options.errorHandler,
		retryPolicyHandler: retryPolicy,
		metrics:            m,
		health:             hc,
		retryAfterMax:      c.options.retryAfterMax,
		attemptTimeout:     c.options.attemptTimeout,
		jar:                jar,
		retryAfterStatuses: c.options.retryAfterStatuses,
	}

	stdClient.Transport = rt

	if c.options.limiter != nil && !perAttempt {
		stdClient.Transport = limiters.NewRoundTripper(stdClient.Transport, c.options.limiter)
	}

	return stdClient
}

// passThroughPolicy is the retry policy of [WithoutRetry]: never retry and
// never turn a reply into an error, so the response (any status) or the
// transport error reaches the caller unchanged.
func passThroughPolicy(context.Context, *http.Response, error) (bool, error) { return false, nil }

// IsUnexpectedStatusError checks if the given error is an UnexpectedStatusError and returns it if found.
// This function unwraps nested errors to find UnexpectedStatusError in the error chain.
// Returns nil if the error is not an UnexpectedStatusError.
//
// Example:
//
//	resp, err := client.Get("https://api.example.com/users")
//	if err != nil {
//	    if statusErr := httpclient.IsUnexpectedStatusError(err); statusErr != nil {
//	        log.Printf("Request failed with status %d", statusErr.Status)
//	        return
//	    }
//	    // Handle other errors
//	}
func IsUnexpectedStatusError(err error) *UnexpectedStatusError {
	target, _ := errors.AsType[*UnexpectedStatusError](err)
	return target
}

// IsCircuitBreakerOpen checks if the given error indicates that the circuit breaker is in the open state.
// When the circuit breaker is open, requests fail immediately without attempting to contact the service.
// This typically happens after a series of failures to protect the service from being overwhelmed.
//
// Example:
//
//	_, err := client.Get("https://api.example.com/users")
//	if err != nil {
//	    if httpclient.IsCircuitBreakerOpen(err) {
//	        log.Println("Service temporarily unavailable, circuit breaker is open")
//	        // Implement fallback logic or return cached data
//	        return
//	    }
//	    // Handle other errors
//	}
func IsCircuitBreakerOpen(err error) bool {
	return errors.Is(err, ErrCircuitBreakerOpen) ||
		errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}
