// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package http provides foundational HTTP utilities and types.
// It complements the standard net/http package with reusable components
// for building resilient HTTP clients and servers.
//
// RoundTripperFunc is stateless and safe for concurrent use.
//
// Import this package under the corehttp alias to avoid shadowing the
// standard library net/http.
//
// # Usage
//
//	rt := http.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
//	    // Clone before mutating: RoundTrippers must not modify the request.
//	    req = req.Clone(req.Context())
//	    req.Header.Set("User-Agent", "go-atlas")
//	    return http.DefaultTransport.RoundTrip(req)
//	})
//
//	client := &http.Client{Transport: rt}
package http
