// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiters

import (
	"cmp"
	"net/http"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// RoundTripper implements [net/http.RoundTripper] by checking a
// [RequestsLimiter] before forwarding each request to the next transport.
// When the limiter denies a request, RoundTrip returns immediately with an
// error and does not contact the server.
type RoundTripper struct {
	next    http.RoundTripper
	limiter RequestsLimiter
}

// NewRoundTripper creates a [RoundTripper] that calls limiter.Allow with the
// request hostname before delegating to next. If next is nil,
// [net/http.DefaultTransport] is used.
//
// Example:
//
//	limiter := &myRateLimiter{limit: 100}
//	rt := limiters.NewRoundTripper(http.DefaultTransport, limiter)
//	client := &http.Client{Transport: rt}
func NewRoundTripper(next http.RoundTripper, limiter RequestsLimiter) *RoundTripper {
	return &RoundTripper{
		next:    cmp.Or(next, http.DefaultTransport),
		limiter: limiter,
	}
}

// RoundTrip checks [RequestsLimiter.Allow] using the request hostname as key.
// If the limiter returns a non-nil error the request is rejected without
// reaching the network; otherwise the request is forwarded to the next
// [net/http.RoundTripper].
func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	err := rt.limiter.Allow(req.Context(), req.URL.Hostname())
	if err != nil {
		return nil, coreerrs.Wrapf(err, "rate limit check failed for %s %s", req.Method, req.URL.String())
	}

	return rt.next.RoundTrip(req)
}
