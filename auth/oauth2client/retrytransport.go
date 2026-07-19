// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/altessa-s/go-atlas/core/retry"
)

// Retry-transport defaults, mirroring the [Exchanger] retry defaults.
const (
	// DefaultTransportRetryAttempts is the number of retries after the first try.
	DefaultTransportRetryAttempts = 2
	// DefaultTransportRetryBaseDelay is the backoff before the first retry.
	DefaultTransportRetryBaseDelay = 200 * time.Millisecond
	// DefaultTransportRetryMaxDelay caps the exponential backoff between retries.
	DefaultTransportRetryMaxDelay = 5 * time.Second
)

// RetryTransport wraps base so requests are retried on transient failures — a
// transport error, HTTP 429, or a 5xx response — with exponential backoff.
// Inject the returned round-tripper via [WithHttpClient] to add retries to the
// x/oauth2-backed grants ([ClientCredentials], [Refresh], [AuthCode],
// [DeviceFlow]), which, unlike [Exchanger], do not retry on their own:
//
//	client := &http.Client{Transport: oauth2client.RetryTransport(http.DefaultTransport)}
//	src := oauth2client.ClientCredentials(ctx, tokenURL, id, secret,
//	    oauth2client.WithHttpClient(client))
//
// A nil base uses [http.DefaultTransport]. Only requests whose body can be
// replayed (GetBody set, as x/oauth2's form posts are) are retried; others get a
// single attempt. On exhausted retries the last response is returned unchanged,
// so the caller still observes the real status and body. 4xx other than 429 are
// returned immediately.
func RetryTransport(base http.RoundTripper, opts ...RetryTransportOption) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	cfg := retryTransportConfig{
		attempts:  DefaultTransportRetryAttempts,
		baseDelay: DefaultTransportRetryBaseDelay,
		maxDelay:  DefaultTransportRetryMaxDelay,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return &retryTransport{
		base:      base,
		attempts:  cfg.attempts,
		nextDelay: retry.Exponential(retry.ExponentialConfig{BaseDelay: cfg.baseDelay, MaxDelay: cfg.maxDelay}),
	}
}

// retryTransportConfig carries the tunables for [RetryTransport].
type retryTransportConfig struct {
	attempts  int
	baseDelay time.Duration
	maxDelay  time.Duration
}

// RetryTransportOption configures [RetryTransport].
type RetryTransportOption func(*retryTransportConfig)

// WithTransportAttempts sets the number of retries after the first attempt.
// A negative value is ignored; zero disables retrying.
func WithTransportAttempts(n int) RetryTransportOption {
	return func(c *retryTransportConfig) {
		if n >= 0 {
			c.attempts = n
		}
	}
}

// WithTransportBackoff bounds the exponential backoff between retries. Non-positive
// values are ignored.
func WithTransportBackoff(base, maxDelay time.Duration) RetryTransportOption {
	return func(c *retryTransportConfig) {
		if base > 0 {
			c.baseDelay = base
		}
		if maxDelay > 0 {
			c.maxDelay = maxDelay
		}
	}
}

// retryTransport retries transient token-endpoint failures.
type retryTransport struct {
	base      http.RoundTripper
	attempts  int
	nextDelay retry.NextDelayFunc
}

// errRetryableStatus signals a retryable HTTP status (429 or 5xx) to the
// [retry.Do] loop; the response itself travels in the RoundTrip closure.
var errRetryableStatus = errors.New("oauth2client: retryable http status")

// RoundTrip retries req on transient failures, replaying the body via GetBody.
// The loop mechanics are delegated to [retry.Do]; the closure classifies the
// outcome (any transport error, HTTP 429, or a 5xx response is retryable) and
// carries the last response across attempts.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A body with no GetBody cannot be replayed safely — do a single attempt.
	if t.attempts <= 0 || (req.Body != nil && req.GetBody == nil) {
		return t.base.RoundTrip(req)
	}

	var (
		lastResp     *http.Response
		replayErr    error
		firstAttempt = true
	)
	retryErr := retry.Do(req.Context(), func(ctx context.Context) error {
		attemptReq := req
		if !firstAttempt {
			attemptReq = req.Clone(ctx)
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					replayErr = err
					return err
				}
				attemptReq.Body = body
			}
		}
		firstAttempt = false

		resp, err := t.base.RoundTrip(attemptReq) //nolint:bodyclose // resp is stored in lastResp and either drained on retry or returned to the caller
		lastResp = resp
		if err != nil {
			return err
		}
		if resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
			return errRetryableStatus
		}
		return nil
	},
		retry.WithMaxAttempts(t.attempts),
		retry.WithShouldRetry(func(error) bool { return replayErr == nil }),
		retry.WithNextDelay(func(attempt int, err error) time.Duration {
			// Exhausted: surface the last response as is, without the trailing
			// backoff sleep (and OnRetry drain) retry.Do would otherwise run
			// after the final failed attempt.
			if attempt >= t.attempts {
				return 0
			}
			return t.nextDelay(attempt, err)
		}),
		retry.WithOnRetry(func(int, error, time.Duration) {
			// Discard the retryable response before the backoff sleep.
			if lastResp != nil {
				_, _ = io.Copy(io.Discard, lastResp.Body)
				_ = lastResp.Body.Close()
				lastResp = nil
			}
		}),
	)

	switch {
	case retryErr == nil, errors.Is(retryErr, errRetryableStatus):
		// Success, a non-retryable status, or exhausted retries on a
		// retryable status: return the last response unchanged.
		return lastResp, nil
	case replayErr != nil:
		return nil, replayErr
	default:
		// A transport error on the last attempt, or context cancellation
		// mid-backoff (the drained response was already cleared).
		return lastResp, retryErr
	}
}
