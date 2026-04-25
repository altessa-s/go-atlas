// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package responder provides unified error response handling for HTTP
// middleware and handlers.
//
// It enables middleware to write structured error responses using the same
// writer + builder pattern as handlers, regardless of whether they call
// [http.Error] or the explicit [WriteError] helper.
//
// The package provides two mechanisms:
//
//  1. Automatic interception: [ErrorInterceptor] wraps [http.ResponseWriter]
//     and buffers error responses (status >= 400). When flushed, the
//     buffered message is rewritten as a structured response via the
//     [ErrorWriter] stored in context. This converts plain [http.Error]
//     calls into content-negotiated JSON/XML responses transparently.
//
//  2. Explicit helper: [WriteError] retrieves the [ErrorWriter] from
//     context and delegates to it, falling back to [http.Error] when no
//     writer is available. Use this when passing typed errors rather than
//     strings.
//
// The [ErrorWriter] is injected into context by the server's error
// interceptor middleware during request setup.
//
// # Example (automatic interception)
//
//	// Any middleware using http.Error automatically gets structured responses
//	http.Error(w, "Rate Limit Exceeded", http.StatusTooManyRequests)
//	// Output: {"error":{"message":"Rate Limit Exceeded"}} (with content negotiation)
//
// # Example (explicit helper)
//
//	// Middleware can use explicit helper for custom errors
//	responder.WriteError(w, r, ErrRateLimitExceeded, http.StatusTooManyRequests)
package responder
