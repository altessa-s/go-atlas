// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package realip provides middleware that extracts the real client IP
// address from request headers and stores it in the request context.
//
// The extraction is delegated to a [clientip.Extractor] which handles
// trusted-proxy validation and header parsing (X-Forwarded-For,
// X-Real-IP, etc.). Downstream middleware and handlers retrieve the IP
// via [FromContext].
//
// This middleware has no dependencies and should be placed early in the
// chain so that other middleware (logger, limiter, tracing) can read
// the real IP from context.
//
// # Example
//
//	extractor, _ := clientip.NewExtractor(
//	    clientip.WithHeaders("X-Real-IP", "X-Forwarded-For"),
//	)
//	handler := realip.Middleware(extractor, logger)(yourHandler)
package realip
