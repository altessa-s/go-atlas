// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import "net/http"

// RoundTripFunc is an adapter that allows ordinary functions to be used as [http.RoundTripper].
// Assign it to an [http.Client].Transport field to intercept outgoing requests in tests.
type RoundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements http.RoundTripper.
func (f RoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
