// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package limiters provides a pluggable rate limiting layer for HTTP clients.
//
// Implement the [RequestsLimiter] interface with any rate limiting strategy
// (token bucket, sliding window, etc.) and wrap an existing
// [net/http.RoundTripper] with [NewRoundTripper] to enforce per-host request
// limits at the transport level. The resulting [RoundTripper] is safe for
// concurrent use and integrates transparently with the resilient client in the
// parent [github.com/altessa-s/go-atlas/transport/http/client] package via
// [client.WithLimiter].
//
// Example:
//
//	rt := limiters.NewRoundTripper(http.DefaultTransport, myLimiter)
//	client := &http.Client{Transport: rt}
package limiters
