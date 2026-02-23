// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/sony/gobreaker/v2"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

const (
	// DefaultRetryWaitMin defines the minimum wait time between retry attempts
	DefaultRetryWaitMin = 500 * time.Millisecond
	// DefaultRetryWaitMax defines the maximum wait time between retry attempts
	DefaultRetryWaitMax = 10 * time.Second
	// DefaultRetryMax defines the default maximum number of retry attempts
	DefaultRetryMax = 5
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
	circuitBreakerClient := newCircuitBreakerClient(c.options)
	retryClient := &retryablehttp.Client{
		HTTPClient:   circuitBreakerClient.standardClient(),
		Logger:       NewLogger(c.options.logger),
		RetryWaitMin: c.options.retryWaitMin,
		RetryWaitMax: c.options.retryWaitMax,
		RetryMax:     c.options.retryMax,
		CheckRetry:   c.retryPolicy,
		Backoff:      retryablehttp.LinearJitterBackoff,
		ErrorHandler: func(resp *http.Response, err error, numTries int) (*http.Response, error) {
			return resp, err
		},
	}

	if c.options.errorHandler != nil {
		retryClient.ErrorHandler = retryablehttp.ErrorHandler(c.options.errorHandler)
	}

	if c.options.retryPolicyHandler != nil {
		retryClient.CheckRetry = retryablehttp.CheckRetry(c.options.retryPolicyHandler)
	}

	client := retryClient.StandardClient()
	if c.options.limiter != nil {
		client.Transport = limiters.NewRoundTripper(client.Transport, c.options.limiter)
	}

	return client
}

// retryPolicy implements the retryablehttp.CheckRetry interface.
// It determines whether a request should be retried based on the response status code
// and any error that occurred.
func (c *Client) retryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	// do not retry on context.Canceled or context.DeadlineExceeded
	if coreerrs.IsContextCanceledOrDeadlineExceeded(ctx.Err()) {
		return false, &NonRetryableError{Err: coreerrs.Wrapf(ctx.Err(), "request context error")}
	}

	if err != nil {
		// Check for SSRF errors — never retry blocked private IPs
		if IsSSRFError(err) != nil {
			return false, &NonRetryableError{Err: err}
		}

		// Check for non-retryable errors using core helpers
		if coreerrs.IsResourceRedirects(err) || coreerrs.IsUnsupportedProtocolScheme(err) ||
			coreerrs.IsCertUnknownAuthority(err) || coreerrs.IsConnectionRefused(err) {
			return false, &NonRetryableError{Err: err}
		}

		// Check for circuit breaker errors
		if IsCircuitBreakerOpen(err) {
			var state string
			if errors.Is(err, gobreaker.ErrOpenState) {
				state = "open"
			} else {
				state = "too_many_requests"
			}
			return false, &CircuitBreakerError{
				Name:  "httpclient",
				State: state,
			}
		}

		// Check for response size errors
		if IsResponseSizeError(err) != nil {
			return false, err // Already typed, don't wrap
		}

		// The error is likely recoverable so retry.
		return true, nil
	}

	if resp.StatusCode == http.StatusBadGateway ||
		resp.StatusCode == http.StatusServiceUnavailable ||
		resp.StatusCode == http.StatusGatewayTimeout ||
		resp.StatusCode == http.StatusRequestTimeout ||
		resp.StatusCode == http.StatusTooManyRequests {
		return true, nil
	}

	if resp.StatusCode == 0 || (resp.StatusCode >= http.StatusBadRequest) {
		return false, &UnexpectedStatusError{
			Status: resp.StatusCode,
			Method: resp.Request.Method,
			Host:   resp.Request.Host,
			URI:    resp.Request.URL.RequestURI(),
		}
	}

	return false, nil
}

// IsUnexpectedStatusError checks if the given error is an UnexpectedStatusError and returns it if found.
// This function unwraps nested errors to find UnexpectedStatusError in the error chain,
// which is necessary because go-retryablehttp wraps errors.
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
	// The go-httpclient package uses hashicorp go-retryablehttp which wraps errors
	// in a chain: fmt.Errorf -> url.Error. We need to unwrap the error chain.
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
