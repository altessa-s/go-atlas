// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package http

import "net/http"

// RoundTripperFunc is an adapter that allows ordinary functions to satisfy the
// [http.RoundTripper] interface. If f is a function with the appropriate
// signature, RoundTripperFunc(f) is a [http.RoundTripper] whose [RoundTripperFunc.RoundTrip]
// method calls f.
//
// RoundTripperFunc is useful for injecting middleware, decorating transports,
// or providing lightweight stubs in tests without declaring a named type.
//
// Because RoundTripperFunc carries no mutable state of its own, it is safe for
// concurrent use by multiple goroutines provided the underlying function is
// also safe for concurrent use.
type RoundTripperFunc func(req *http.Request) (*http.Response, error)

// RoundTrip executes the underlying function with req and returns its results,
// implementing the [http.RoundTripper] interface. It propagates any error
// returned by the wrapped function unchanged.
func (rt RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return rt(req)
}
