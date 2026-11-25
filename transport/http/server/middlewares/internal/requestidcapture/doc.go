// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package requestidcapture makes an inner middleware's resolved request ID
// available to the outer HTTP recovery handler.
//
// # Usage
//
//	ctx := requestidcapture.WithCapture(r.Context())
//	next.ServeHTTP(w, r.WithContext(ctx))
//
// Request ID middleware calls [Record] on its derived context. Recovery reads
// [ID] on the original capture when a panic occurs.
package requestidcapture
