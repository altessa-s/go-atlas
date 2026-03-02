// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sony/gobreaker/v2"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/runtime/retry"
)

// retryableStatusError is an internal sentinel indicating a retryable HTTP status.
type retryableStatusError struct{ status int }

func (e *retryableStatusError) Error() string {
	return fmt.Sprintf("retryable HTTP status: %d", e.status)
}

// retryRoundTripper wraps an [http.RoundTripper] with retry logic powered by
// [coreretry.Do]. It replaces the former retryablehttp.Client approach with a
// composable RoundTripper that sits in the standard transport chain.
type retryRoundTripper struct {
	next               http.RoundTripper
	cfg                coreretry.Config
	logger             *slog.Logger
	errorHandler       ErrorHandler
	retryPolicyHandler RetryPolicyHandler
}

// RoundTrip implements [http.RoundTripper]. On the first call it buffers the
// request body (if any) so it can be replayed on retries, then delegates retry
// orchestration to [coreretry.Do].
func (rt *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, coreerrs.Wrapf(err, "failed to read request body for retry buffering")
		}
	}

	var (
		lastResp *http.Response
		lastErr  error
	)

	ctx := req.Context()

	cfg := rt.cfg
	cfg.ShouldRetry = func(err error) bool {
		return rt.shouldRetry(err)
	}

	if rt.logger != nil {
		cfg.OnRetry = func(attempt int, err error, delay time.Duration) {
			rt.logger.DebugContext(ctx,
				"http client retrying request",
				slog.Int("attempt", attempt),
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.String("error", err.Error()),
				slog.Duration("delay", delay),
			)
		}
	}

	retryErr := coreretry.Do(ctx, cfg, func(ctx context.Context) error {
		// Close previous response body if present from a prior attempt.
		if lastResp != nil {
			_ = lastResp.Body.Close()
			lastResp = nil
		}

		// Replay buffered body.
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			req.ContentLength = int64(len(bodyBytes))
			req.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(bodyBytes)), nil
			}
		}

		resp, err := rt.next.RoundTrip(req) //nolint:bodyclose // resp is stored in lastResp and either closed on next retry or returned to caller
		lastResp = resp
		lastErr = err

		// If user supplied a custom RetryPolicyHandler, delegate to it.
		if rt.retryPolicyHandler != nil {
			retry, policyErr := rt.retryPolicyHandler(ctx, resp, err)
			if policyErr != nil {
				lastErr = policyErr
				if !retry {
					return &NonRetryableError{Err: policyErr}
				}
				return policyErr
			}
			if !retry {
				return nil // signal success to stop retrying
			}
			if err != nil {
				return err
			}
			// retry == true && err == nil → retryable status on resp
			if resp != nil {
				lastErr = &retryableStatusError{status: resp.StatusCode}
				return lastErr
			}
			lastErr = nil
			return nil
		}

		// Default classification.
		if err != nil {
			return classifyTransportError(ctx, err)
		}

		classified := classifyResponse(resp)
		lastErr = classified
		return classified
	})

	// Handle retry exhaustion.
	if retryErr != nil && lastResp == nil {
		if rt.errorHandler != nil {
			return rt.errorHandler(nil, retryErr, rt.cfg.MaxAttempts+1)
		}
		return nil, retryErr
	}

	if retryErr != nil {
		// We have a response but also an error (e.g., retryable status exhausted).
		if rt.errorHandler != nil {
			return rt.errorHandler(lastResp, retryErr, rt.cfg.MaxAttempts+1)
		}
		return lastResp, retryErr
	}

	return lastResp, lastErr
}

// shouldRetry returns false for non-retryable error types.
func (rt *retryRoundTripper) shouldRetry(err error) bool {
	if _, ok := coreerrs.AsType[*NonRetryableError](err); ok {
		return false
	}
	if _, ok := coreerrs.AsType[*CircuitBreakerError](err); ok {
		return false
	}
	if _, ok := coreerrs.AsType[*UnexpectedStatusError](err); ok {
		return false
	}
	_, ok := coreerrs.AsType[*ResponseSizeError](err)
	return !ok
}

// classifyTransportError examines a transport-level error and wraps it as
// non-retryable when appropriate, matching the former retryPolicy logic.
func classifyTransportError(ctx context.Context, err error) error {
	if coreerrs.IsContextCanceledOrDeadlineExceeded(ctx.Err()) {
		return &NonRetryableError{Err: coreerrs.Wrapf(ctx.Err(), "request context error")}
	}

	if IsSSRFError(err) != nil {
		return &NonRetryableError{Err: err}
	}

	if coreerrs.IsResourceRedirects(err) || coreerrs.IsUnsupportedProtocolScheme(err) ||
		coreerrs.IsCertUnknownAuthority(err) || coreerrs.IsConnectionRefused(err) {
		return &NonRetryableError{Err: err}
	}

	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		var state string
		if errors.Is(err, gobreaker.ErrOpenState) {
			state = "open"
		} else {
			state = "too_many_requests"
		}
		return &CircuitBreakerError{
			Name:  "httpclient",
			State: state,
		}
	}

	if IsResponseSizeError(err) != nil {
		return err
	}

	// The error is likely recoverable so retry.
	return err
}

// classifyResponse examines an HTTP response and returns nil for success, a
// retryable error for transient status codes, or an [UnexpectedStatusError]
// for non-retryable failures.
func classifyResponse(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusRequestTimeout,
		http.StatusTooManyRequests:
		return &retryableStatusError{status: resp.StatusCode}
	}

	if resp.StatusCode == 0 || resp.StatusCode >= http.StatusBadRequest {
		return &UnexpectedStatusError{
			Status: resp.StatusCode,
			Method: resp.Request.Method,
			Host:   resp.Request.Host,
			URI:    resp.Request.URL.RequestURI(),
		}
	}

	return nil
}
