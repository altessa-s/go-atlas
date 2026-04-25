// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tracing provides HTTP middleware for distributed tracing.
//
// The middleware integrates with the observability/tracing package to provide:
//   - Automatic span creation for each HTTP request
//   - W3C Trace Context propagation via HTTP headers
//   - Span attributes for HTTP method, URL, status code, and client information
//   - Error recording for 4xx and 5xx responses
//
// # Basic Usage
//
//	m := tracing.New(tracer)
//	handler := m.Handler(myHandler)
//
// # With Options
//
//	m := tracing.New(tracer,
//	    tracing.WithIgnorePaths("/health", "/metrics"),
//	    tracing.WithIgnorePatterns(`^/static/.*`),
//	    tracing.WithLogger(logger),
//	)
//	handler := m.Handler(myHandler)
//
// # Span Attributes
//
// The following attributes are automatically added to spans:
//   - http.method: Request method (GET, POST, etc.)
//   - http.url: Full request URL
//   - http.target: Request path
//   - http.host: Request host
//   - http.scheme: Request scheme (http/https)
//   - http.status_code: Response status code
//   - http.user_agent: User agent string
//   - net.peer.ip: Client IP address
//
// # Context Propagation
//
// The middleware extracts trace context from incoming requests using W3C Trace Context
// headers (traceparent, tracestate). For outgoing requests, inject trace context
// using the propagator's Inject method.
package tracing
