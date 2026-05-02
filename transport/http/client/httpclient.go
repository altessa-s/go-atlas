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

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
)

const (
	// DefaultRetryWaitMin defines the minimum wait time between retry attempts
	DefaultRetryWaitMin = 500 * time.Millisecond
	// DefaultRetryWaitMax defines the maximum wait time between retry attempts
	DefaultRetryWaitMax = 10 * time.Second
	// DefaultRetryMax defines the default maximum number of retry attempts
	DefaultRetryMax = 5

	// defaultBackoffFactor is the exponential backoff multiplier for retry delays.
	defaultBackoffFactor = 1.5
	// defaultBackoffJitter is the fraction of jitter added to retry delays.
	defaultBackoffJitter = 0.25
)

// ErrorHandler defines the callback for intercepting and handling errors during the retry process.
// Set it via [WithErrorHandler]. The handler receives the HTTP response (may be nil), the error
// that occurred, and the number of retry attempts made. It can modify or wrap the error before
// returning, allowing for custom error handling logic.
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
	m := newHTTPClientMetrics(c.options.collector, c.options.metricsSubsystem)
	cbClient := newCircuitBreakerClient(c.options, m)
	stdClient := cbClient.standardClient()

	retryOpts := []coreretry.Option{
		coreretry.WithMaxAttempts(c.options.retryMax),
		coreretry.WithNextDelay(coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: c.options.retryWaitMin,
			MaxDelay:  c.options.retryWaitMax,
			Factor:    defaultBackoffFactor,
			Jitter:    defaultBackoffJitter,
		})),
	}

	rt := &retryRoundTripper{
		next:               stdClient.Transport,
		retryOpts:          retryOpts,
		maxAttempts:        c.options.retryMax,
		logger:             c.options.logger,
		errorHandler:       c.options.errorHandler,
		retryPolicyHandler: c.options.retryPolicyHandler,
		metrics:            m,
	}

	stdClient.Transport = rt

	if c.options.limiter != nil {
		stdClient.Transport = limiters.NewRoundTripper(stdClient.Transport, c.options.limiter)
	}

	return stdClient
}

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
	target, _ := coreerrs.AsType[*UnexpectedStatusError](err)
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
	return errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)
}
