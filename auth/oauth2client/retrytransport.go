// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oauth2client

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync/atomic"
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
// returned immediately. Refresh tokens, authorization codes and device codes
// are only retried after a proven dial or TLS handshake failure by default.
// [WithInsecureRetrySingleUseGrants] enables replay after ambiguous failures.
func RetryTransport(base http.RoundTripper, opts ...RetryTransportOption) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	cfg := newRetryTransportOptions(opts...)
	return &retryTransport{
		base:                         base,
		attempts:                     cfg.attempts,
		insecureRetrySingleUseGrants: cfg.insecureRetrySingleUseGrants,
		nextDelay: retry.Exponential(
			cfg.baseDelay,
			retry.WithExponentialMaxDelay(cfg.maxDelay),
			retry.WithExponentialJitter(DefaultRetryJitter),
		),
	}
}

// WithTransportAttempts sets the number of retries after the first attempt.
// A negative value is ignored; zero disables retrying.
func WithTransportAttempts(n int) RetryTransportOption {
	return func(c *retryTransportOptions) {
		if n >= 0 {
			c.attempts = n
		}
	}
}

// WithTransportBackoff bounds the exponential backoff between retries. Non-positive
// values are ignored.
func WithTransportBackoff(base, maxDelay time.Duration) RetryTransportOption {
	return func(c *retryTransportOptions) {
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
	base                         http.RoundTripper
	attempts                     int
	nextDelay                    retry.NextDelayFunc
	insecureRetrySingleUseGrants bool
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

// RoundTrip retries eligible failures, replaying the body via GetBody.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// A body that cannot be replayed gets a single attempt.
	if t.attempts <= 0 || !replayable(req) {
		return t.roundTrip(req)
	}

	singleUse := !t.insecureRetrySingleUseGrants && singleUseGrant(req)

	var (
		lastResp     *http.Response
		replayErr    error
		preWrite     bool
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

		var trace *grantAttemptTrace
		if singleUse {
			trace = &grantAttemptTrace{}
			attemptReq = trace.attach(attemptReq)
		}
		resp, err := t.roundTrip(attemptReq) //nolint:bodyclose // resp is stored in lastResp and either drained on retry or returned to the caller
		lastResp = resp
		if err != nil {
			preWrite = singleUse && resp == nil && req.Response == nil && trace.failedBeforeWrite(err)
			return err
		}
		if !singleUse && resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) {
			return errRetryableStatus
		}
		return nil
	},
		retry.WithMaxAttempts(t.attempts),
		retry.WithShouldRetry(func(err error) bool {
			return replayErr == nil && !permanentRequestError(err) && (!singleUse || preWrite)
		}),
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

// singleUseGrant inspects only a replay copy and caps the work. An
// unreadable or malformed OAuth form is handled conservatively.
func singleUseGrant(req *http.Request) bool {
	if req.Method != http.MethodPost || req.GetBody == nil {
		return false
	}
	mediaType, _, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if mediaType != "application/x-www-form-urlencoded" {
		return false
	}
	body, err := req.GetBody()
	if err != nil || body == nil {
		return true
	}
	defer func() { _ = body.Close() }()
	const maxFormBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(body, maxFormBytes+1))
	if err != nil || len(data) > maxFormBytes {
		return true
	}
	form, err := url.ParseQuery(string(data))
	if err != nil {
		return true
	}
	for _, grant := range form["grant_type"] {
		switch grant {
		case "refresh_token", "authorization_code", "urn:ietf:params:oauth:grant-type:device_code":
			return true
		}
	}
	return false
}

type grantAttemptTrace struct {
	gotConn    atomic.Bool
	wrote      atomic.Bool
	readBody   atomic.Bool
	dialFailed atomic.Bool
	tlsFailed  atomic.Bool
}

func (t *grantAttemptTrace) attach(req *http.Request) *http.Request {
	trace := &httptrace.ClientTrace{
		GotConn:      func(httptrace.GotConnInfo) { t.gotConn.Store(true) },
		WroteHeaders: func() { t.wrote.Store(true) },
		WroteRequest: func(httptrace.WroteRequestInfo) { t.wrote.Store(true) },
		ConnectDone: func(_, _ string, err error) {
			if err != nil {
				t.dialFailed.Store(true)
			}
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			if err != nil {
				t.tlsFailed.Store(true)
			}
		},
	}
	attempt := req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	attempt.Body = t.trackBody(req.Body)
	attempt.GetBody = func() (io.ReadCloser, error) {
		body, err := req.GetBody()
		return t.trackBody(body), err
	}
	return attempt
}

func (t *grantAttemptTrace) trackBody(body io.ReadCloser) io.ReadCloser {
	if body == nil || body == http.NoBody {
		return body
	}
	return &grantAttemptBody{ReadCloser: body, read: &t.readBody}
}

type grantAttemptBody struct {
	io.ReadCloser
	read *atomic.Bool
}

func (b *grantAttemptBody) Read(p []byte) (int, error) {
	b.read.Store(true)
	return b.ReadCloser.Read(p)
}

func (t *grantAttemptTrace) failedBeforeWrite(err error) bool {
	if t.gotConn.Load() || t.wrote.Load() || t.readBody.Load() {
		return false
	}
	op, ok := errors.AsType[*net.OpError](err)
	return t.tlsFailed.Load() || (t.dialFailed.Load() && ok && op.Op == "dial")
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
