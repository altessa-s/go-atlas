// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
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

// RoundTrip retries req on transient failures, replaying the body via GetBody.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A body with no GetBody cannot be replayed safely — do a single attempt.
	if t.attempts <= 0 || (req.Body != nil && req.GetBody == nil) {
		return t.base.RoundTrip(req)
	}

	for attempt := 0; ; attempt++ {
		attemptReq := req
		if attempt > 0 {
			attemptReq = req.Clone(req.Context())
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return nil, err
				}
				attemptReq.Body = body
			}
		}

		resp, err := t.base.RoundTrip(attemptReq)
		retryable := err != nil || (resp != nil &&
			(resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500))
		if !retryable || attempt >= t.attempts {
			return resp, err
		}

		// Discard the retryable response before the next attempt.
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		delay := t.nextDelay(attempt, err)
		timer := time.NewTimer(delay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
}
