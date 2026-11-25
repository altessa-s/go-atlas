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

	"github.com/altessa-s/go-atlas-core/retry"
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
// Install it on an HTTP client and pass the client through the grant's HTTPClient
// option to retry requests from [ClientCredentials], [Refresh], [AuthCode],
// or [DeviceFlow]:
//
//	client := &http.Client{Transport: oauth2client.RetryTransport(http.DefaultTransport)}
//	src, err := oauth2client.ClientCredentials(ctx, tokenURL, id, secret,
//	    oauth2client.WithClientCredentialsHTTPClient(client))
//
// A nil base uses [http.DefaultTransport]. Only requests that can be sent twice
// are retried — one with no body, one carrying [http.NoBody], or one whose
// GetBody is set (as x/oauth2's form posts are); anything else gets a single
// attempt. On exhausted retries the last response is returned unchanged, so the
// caller still observes the real status and body. 4xx other than 429 are
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
		base:     base,
		attempts: cfg.attempts,
		nextDelay: retry.Exponential(
			cfg.baseDelay,
			retry.WithExponentialMaxDelay(cfg.maxDelay),
			retry.WithExponentialJitter(DefaultRetryJitter),
		),
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

// replayable reports whether req can be sent more than once.
//
// A request with no body, or one carrying [http.NoBody], is trivially
// replayable: there is nothing to re-read. The [http.NoBody] case has to be
// named explicitly because [http.NewRequest] leaves GetBody nil for it — the
// body is already empty, so it sees no reason to synthesize a getter — and a
// "GetBody != nil" test alone therefore reads the most idiomatic way of saying
// "this request has no body" as "this body cannot be replayed", silently
// costing every such request its retries.
func replayable(req *http.Request) bool {
	return req.Body == nil || req.Body == http.NoBody || req.GetBody != nil
}

// RoundTrip retries req on transient failures, replaying the body via GetBody.
// The loop mechanics are delegated to [retry.Do]; the closure classifies the
// outcome (any transport error, HTTP 429, or a 5xx response is retryable) and
// carries the last response across attempts.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A body that cannot be replayed gets a single attempt.
	if t.attempts <= 0 || !replayable(req) {
		return t.roundTrip(req)
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
					replayErr = preResponseError(err)
					return replayErr
				}
				attemptReq.Body = body
			}
		}
		firstAttempt = false

		resp, err := t.roundTrip(attemptReq) //nolint:bodyclose // resp is stored in lastResp and either drained on retry or returned to the caller
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
		retry.WithShouldRetry(func(err error) bool { return replayErr == nil && !permanentRequestError(err) }),
		retry.WithNextDelay(t.nextDelay),
		retry.WithOnRetry(func(int, error, time.Duration) {
			// Discard the retryable response before the backoff sleep.
			if lastResp != nil {
				_, _ = io.Copy(io.Discard, lastResp.Body)
				_ = lastResp.Body.Close()
				lastResp = nil
			}
		}),
	)

	// If cancellation prevented the first attempt, the base transport never
	// took ownership of the body. RoundTripper must still close it.
	if firstAttempt && req.Body != nil {
		_ = req.Body.Close()
	}

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
		return lastResp, preResponseError(retryErr)
	}
}

func (t *retryTransport) roundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	response := resp
	if response == nil {
		response = req.Response
	}
	metadata := &responseMetadata{}
	if response != nil {
		metadata.received.Store(true)
		metadata.status.Store(int64(response.StatusCode))
	}
	return resp, metadata.redact(err)
}
