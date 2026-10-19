// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/altessa-s/go-atlas/transport/http/client/limiters"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreio "github.com/altessa-s/go-atlas/core/io"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
)

// statusToClass converts an HTTP status code to a class string (2xx, 3xx, 4xx, 5xx).
func statusToClass(code int) string {
	switch {
	case code >= http.StatusOK && code < http.StatusMultipleChoices:
		return "2xx"
	case code >= http.StatusMultipleChoices && code < http.StatusBadRequest:
		return "3xx"
	case code >= http.StatusBadRequest && code < http.StatusInternalServerError:
		return "4xx"
	case code >= http.StatusInternalServerError:
		return "5xx"
	default:
		return "unknown"
	}
}

// retryableStatusError is an internal sentinel indicating a retryable HTTP
// status. retryAfter is the pause the server asked for (429/503 Retry-After,
// capped), zero when absent.
type retryableStatusError struct {
	status     int
	retryAfter time.Duration
}

func (e *retryableStatusError) Error() string {
	return fmt.Sprintf("retryable HTTP status: %d", e.status)
}

// retryRoundTripper wraps an [http.RoundTripper] with retry logic powered by
// [coreretry.Do]. It replaces the former retryablehttp.Client approach with a
// composable RoundTripper that sits in the standard transport chain.
type retryRoundTripper struct {
	next               http.RoundTripper
	retryOpts          []coreretry.Option // base retry options (cap, delay policy)
	maxAttempts        int                // mirrors WithMaxAttempts for metrics/error reporting
	logger             *slog.Logger
	errorHandler       ErrorHandler
	retryPolicyHandler RetryPolicyHandler
	metrics            *httpClientMetrics
	health             *httpClientHealth
	// attemptTimeout bounds every attempt (limiter wait under
	// WithLimiterPerAttempt, connection, reply and body); non-positive leaves
	// attempts bounded by the request context only.
	attemptTimeout time.Duration
	// jar, when set, is owned by the retry layer (http.Client.Jar is nil):
	// every attempt sends the caller's Cookie header plus the jar's cookies
	// for the URL, and every reply's cookies are stored once, on receipt.
	jar http.CookieJar
	// retryAfterMax caps an honored Retry-After; non-positive ignores it.
	retryAfterMax time.Duration
	// retryAfterStatuses are the statuses whose Retry-After is honored.
	retryAfterStatuses []int
}

// retryableStatus builds the retryable error of resp, carrying its
// Retry-After when the round tripper honors it for that status.
func (rt *retryRoundTripper) retryableStatus(resp *http.Response) *retryableStatusError {
	e := &retryableStatusError{status: resp.StatusCode}
	if rt.retryAfterMax > 0 && honorsRetryAfter(rt.retryAfterStatuses, resp.StatusCode) {
		e.retryAfter, _ = ParseRetryAfter(resp.Header, time.Now(), rt.retryAfterMax)
	}
	return e
}

// retryAfterDelay wraps backoff so a retry after a 429/503 that carried
// Retry-After waits at least as long as the server asked.
func retryAfterDelay(backoff coreretry.NextDelayFunc) coreretry.NextDelayFunc {
	return func(attempt int, err error) time.Duration {
		d := backoff(attempt, err)
		if rs, ok := errors.AsType[*retryableStatusError](err); ok && d > 0 {
			d = max(d, rs.retryAfter)
		}
		return d
	}
}

// attemptRequest returns the request of one attempt: with a cookie jar, a
// copy whose Cookie header is the caller's original one (callerCookie,
// captured before any attempt) plus the jar's cookies for the URL read now;
// with an attempt timeout, a copy under its own deadline, whose cancel func
// is returned.
func (rt *retryRoundTripper) attemptRequest(
	ctx context.Context, req *http.Request, callerCookie []string,
) (*http.Request, context.CancelFunc) {
	out := req
	if rt.jar != nil {
		out = req.Clone(ctx)
		out.Header.Del("Cookie")
		for _, v := range callerCookie {
			out.Header.Add("Cookie", v)
		}
		for _, c := range rt.jar.Cookies(cookieURL(req)) {
			out.AddCookie(c)
		}
	}
	if rt.attemptTimeout <= 0 {
		return out, nil
	}
	actx, cancel := context.WithTimeout(ctx, rt.attemptTimeout)
	return out.WithContext(actx), cancel
}

// cookieURL is the URL cookies of req are looked up and stored under, as in
// net/http's Client.send: req.URL with req.Host as the authority when set.
func cookieURL(req *http.Request) *url.URL {
	if req.Host == "" {
		return req.URL
	}
	u := *req.URL
	if req.URL.User != nil {
		user := *req.URL.User
		u.User = &user
	}
	u.Host = req.Host
	return &u
}

// filterRedirectCookies mirrors net/http's makeHeadersCopier for a jar. It
// runs from the client's CheckRedirect (see redirectPolicy), after
// http.Client copied the initial request's headers into req and before the
// caller's redirect policy: http.Client's own Jar is nil, so it forwarded the
// initial Cookie header unchanged; with a jar it would drop every initial
// cookie whose name a redirect reply of the chain set (replaced or deleted,
// regardless of domain and path) and rebuild the header sorted. The replies
// are req.Response and the Response of every later request in via.
func filterRedirectCookies(req *http.Request, via []*http.Request) {
	if len(via) == 0 || via[0].Header.Get("Cookie") == "" || len(req.Header.Values("Cookie")) == 0 {
		return // no initial cookies, or http.Client stripped them (other domain)
	}
	set := make(map[string]struct{})
	addCookieNames(set, req.Response)
	for _, r := range via[1:] {
		addCookieNames(set, r.Response)
	}
	if len(set) == 0 {
		return
	}
	var kept []string
	changed := false
	for _, c := range via[0].Cookies() {
		if _, ok := set[c.Name]; ok {
			changed = true
			continue
		}
		kept = append(kept, c.Name+"="+c.Value)
	}
	if !changed {
		return
	}
	slices.Sort(kept) // as net/http, for a deterministic header
	req.Header.Set("Cookie", strings.Join(kept, "; "))
}

// addCookieNames adds the names of the cookies resp sets to set.
func addCookieNames(set map[string]struct{}, resp *http.Response) {
	if resp == nil {
		return
	}
	for _, c := range resp.Cookies() {
		set[c.Name] = struct{}{}
	}
}

// maxRedirects is net/http's default redirect limit.
const maxRedirects = 10

// redirectPolicy wraps the client's CheckRedirect for a jar owned by the
// retry layer: the initial cookies are filtered first (filterRedirectCookies),
// then the caller's policy runs — or net/http's default, which stops after 10
// redirects — so a cookie the policy sets on req is kept.
func redirectPolicy(check func(*http.Request, []*http.Request) error) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		filterRedirectCookies(req, via)
		if check != nil {
			return check(req, via)
		}
		if len(via) >= maxRedirects {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
}

// storeCookies saves the cookies resp set for req's URL in the jar. It runs
// once per reply, when the reply is received.
func (rt *retryRoundTripper) storeCookies(req *http.Request, resp *http.Response) {
	if rt.jar == nil || resp == nil {
		return
	}
	if rc := resp.Cookies(); len(rc) > 0 {
		rt.jar.SetCookies(cookieURL(req), rc)
	}
}

// releaseWithBody ties an attempt's cancel func to its reply body, so the
// attempt deadline covers reading the body; on failure it cancels at once.
func releaseWithBody(resp *http.Response, err error, cancel context.CancelFunc) *http.Response {
	if cancel == nil {
		return resp
	}
	if err != nil || resp == nil || resp.Body == nil {
		cancel()
		return resp
	}
	resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp
}

// cancelOnClose releases an attempt's deadline when its body is closed.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

// Close closes the body and releases the attempt deadline.
func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

// CloseIdleConnections forwards to the wrapped transport so
// [http.Client.CloseIdleConnections] releases pooled connections.
func (rt *retryRoundTripper) CloseIdleConnections() { closeIdleConnections(rt.next) }

// RoundTrip implements [http.RoundTripper]. On the first call it buffers the
// request body (if any) so it can be replayed on retries, then delegates retry
// orchestration to [coreretry.Do].
func (rt *retryRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.metrics == nil {
		rt.metrics = newHTTPClientMetrics(nil, "")
	}

	// Body replay strategy: prefer the caller-provided GetBody (net/http sets
	// it automatically for bytes/strings readers); buffer only when retries
	// are possible (maxAttempts counts retries; negative means unlimited) and
	// no replay source exists.
	if req.Body != nil && req.GetBody == nil && rt.maxAttempts != 0 {
		buf := coreio.GetBuffer()
		_, err := buf.ReadFrom(req.Body)
		_ = req.Body.Close()
		if err != nil {
			coreio.PutBuffer(buf)
			return nil, coreerrs.Wrapf(err, "failed to read request body for retry buffering")
		}
		bodyBytes := bytes.Clone(buf.Bytes())
		coreio.PutBuffer(buf)

		req.ContentLength = int64(len(bodyBytes))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(bodyBytes)), nil
		}
		// The original body was consumed by buffering; install the replay
		// copy for the first attempt.
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	var (
		lastResp *http.Response
		lastErr  error
	)

	ctx := req.Context()
	method := req.Method
	stop := rt.metrics.durationFor(method).Start()
	defer stop()

	var retryAttempts int
	perRequestOpts := append(make([]coreretry.Option, 0, len(rt.retryOpts)+2), rt.retryOpts...)
	perRequestOpts = append(perRequestOpts,
		// Method value instead of a wrapper closure: the predicate itself is
		// static per round-tripper.
		coreretry.WithShouldRetry(rt.shouldRetry),
		coreretry.WithOnRetry(func(attempt int, err error, delay time.Duration) {
			retryAttempts++
			// A retry follows: release the discarded response before the
			// backoff sleep so its connection returns to the pool meanwhile.
			// With an ErrorHandler the response must stay readable in case
			// the context ends during the sleep and the handler receives it.
			if rt.errorHandler == nil && lastResp != nil && lastResp.Body != nil {
				closeBody(lastResp)
				lastResp.Body = http.NoBody
			}
			rt.metrics.retries.Inc()
			if rt.logger != nil {
				rt.logger.DebugContext(ctx,
					"http client retrying request",
					slog.Int("attempt", attempt),
					slog.String("method", req.Method),
					// Omit the query string: outbound URLs may carry signed-URL
					// credentials or tokens that must not reach debug logs.
					slog.String("url", req.URL.Scheme+"://"+req.URL.Host+req.URL.Path),
					slog.String("error", err.Error()),
					slog.Duration("delay", delay),
				)
			}
		}),
	)

	// The caller's Cookie header as received: with a jar the retry layer
	// owns cookie handling (http.Client.Jar is nil), so nothing was injected;
	// on a redirect hop it was already filtered by redirectPolicy.
	callerCookie := req.Header.Values("Cookie")

	firstAttempt := true
	retryErr := coreretry.Do(ctx, func(ctx context.Context) error {
		// Close previous response body if present from a prior attempt.
		closeBody(lastResp)
		lastResp = nil

		// Rewind the body for retry attempts; the first attempt uses the
		// body installed above (or the caller's original one).
		if !firstAttempt && req.GetBody != nil {
			body, gbErr := req.GetBody()
			if gbErr != nil {
				lastErr = gbErr
				return &NonRetryableError{Err: coreerrs.Wrapf(gbErr, "failed to rewind request body for retry")}
			}
			req.Body = body
		}
		firstAttempt = false

		attemptReq, cancel := rt.attemptRequest(ctx, req, callerCookie)
		resp, err := rt.next.RoundTrip(attemptReq) //nolint:bodyclose // resp is stored in lastResp and either closed on next retry or returned to caller
		resp = releaseWithBody(resp, err, cancel)  //nolint:bodyclose // same response, closed like resp above
		// Every reply's cookies are stored exactly once, on receipt, whatever
		// happens to the reply next (retried, returned, discarded).
		rt.storeCookies(req, resp)
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
				lastErr = rt.retryableStatus(resp)
				return lastErr
			}
			lastErr = nil
			return nil
		}

		// Default classification.
		if err != nil {
			return classifyTransportError(ctx, err)
		}

		classified := rt.classifyResponse(resp)
		lastErr = classified
		return classified
	}, perRequestOpts...)

	statusClass := "unknown"
	if lastResp != nil {
		statusClass = statusToClass(lastResp.StatusCode)
	}
	rt.metrics.totalFor(method, statusClass).Inc()
	rt.health.recordRequest(req.URL.Hostname(), retryAttempts > 0)

	if retryErr != nil && ctx.Err() == nil && retryable(retryErr) {
		retryErr = exhausted(req, lastResp, retryErr, retryAttempts+1)
	}
	return rt.finish(method, lastResp, lastErr, retryErr)
}

// exhausted returns the [RetryExhaustedError] of a request whose retry loop
// stopped on a retryable failure because no further attempt was allowed (the
// attempts, the elapsed-time budget or the context deadline ran out). An
// exhausted retryable status is reported as an [UnexpectedStatusError] of
// the last reply, so callers classify it like any other failed status. The
// URL omits the query string and user info, which may carry credentials.
func exhausted(req *http.Request, lastResp *http.Response, retryErr error, attempts int) error {
	last := retryErr
	if _, ok := errors.AsType[*retryableStatusError](retryErr); ok && lastResp != nil && lastResp.Request != nil {
		last = unexpectedStatus(lastResp)
	}
	return &RetryExhaustedError{
		LastError: last,
		Attempts:  attempts,
		Method:    req.Method,
		URL:       (&url.URL{Scheme: req.URL.Scheme, Host: req.URL.Host, Path: req.URL.Path}).String(),
	}
}

// unexpectedStatus describes the failed reply resp. A redirect hop's request
// carries no Host field, so the URL's host stands in for it.
func unexpectedStatus(resp *http.Response) *UnexpectedStatusError {
	return &UnexpectedStatusError{
		Status: resp.StatusCode,
		Method: resp.Request.Method,
		Host:   cmp.Or(resp.Request.Host, resp.Request.URL.Host),
		URI:    resp.Request.URL.RequestURI(),
	}
}

// finish turns the outcome of the retry loop into the round trip result.
func (rt *retryRoundTripper) finish(method string, lastResp *http.Response, lastErr, retryErr error) (*http.Response, error) {
	// Handle retry exhaustion and terminal errors (possibly with a response,
	// e.g. a non-retryable or exhausted retryable status).
	if retryErr != nil {
		rt.metrics.errorsFor(method).Inc()
		if rt.errorHandler != nil {
			resp, err := rt.errorHandler(lastResp, retryErr, rt.maxAttempts+1)
			if resp != lastResp {
				// The handler did not hand the original response on.
				closeBody(lastResp)
			}
			return discardOnError(resp, err)
		}
		return discardOnError(lastResp, retryErr)
	}

	return discardOnError(lastResp, lastErr)
}

// discardOnError enforces the [http.RoundTripper] contract that a response
// accompanies only a nil error: [http.Client] drops a response returned
// together with an error without closing it, so its body would leak.
func discardOnError(resp *http.Response, err error) (*http.Response, error) {
	if err != nil {
		closeBody(resp)
		return nil, err
	}
	return resp, nil
}

// maxDiscardDrain caps how many bytes of a discarded response body are read
// before closing it. Draining lets HTTP/1.1 reuse the connection; larger
// bodies are cheaper to abandon together with their connection.
const maxDiscardDrain = 64 << 10

// closeBody drains (up to [maxDiscardDrain]) and closes resp's body when
// there is one. Bodies of a larger known length and protocol upgrades are
// closed without reading, and so is every body of a request without a
// deadline: a stalled upstream must not block the caller indefinitely.
func closeBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	if drainable(resp) {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDiscardDrain))
	}
	_ = resp.Body.Close()
}

// drainable reports whether resp's discarded body is worth draining.
func drainable(resp *http.Response) bool {
	if resp.StatusCode == http.StatusSwitchingProtocols || resp.ContentLength > maxDiscardDrain || resp.Request == nil {
		return false
	}
	_, ok := resp.Request.Context().Deadline()
	return ok
}

// shouldRetry returns false for non-retryable error types, counting a
// circuit breaker trip.
func (rt *retryRoundTripper) shouldRetry(err error) bool {
	if _, ok := errors.AsType[*CircuitBreakerError](err); ok {
		rt.metrics.circuitBreakerTrips.Inc()
		return false
	}
	return retryable(err)
}

// retryable reports whether err is a failure the retry loop retries: not a
// [NonRetryableError], [CircuitBreakerError], [UnexpectedStatusError] or
// [ResponseSizeError].
func retryable(err error) bool {
	if _, ok := errors.AsType[*NonRetryableError](err); ok {
		return false
	}
	if _, ok := errors.AsType[*CircuitBreakerError](err); ok {
		return false
	}
	if _, ok := errors.AsType[*UnexpectedStatusError](err); ok {
		return false
	}
	_, ok := errors.AsType[*ResponseSizeError](err)
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

	// A limiter refusal (WithLimiterPerAttempt): another attempt would be
	// refused or would wait again.
	if errors.Is(err, limiters.ErrRateLimited) {
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
func (rt *retryRoundTripper) classifyResponse(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusRequestTimeout,
		http.StatusTooManyRequests:
		return rt.retryableStatus(resp)
	}

	if resp.StatusCode == 0 || resp.StatusCode >= http.StatusBadRequest {
		return unexpectedStatus(resp)
	}

	return nil
}
