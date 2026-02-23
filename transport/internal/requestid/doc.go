// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package requestid extracts, validates, and generates UUID v4 request
// identifiers from HTTP headers or gRPC metadata.
//
// [Generator] reads a configurable header/metadata key, validates the
// value as a UUID v4, and optionally generates a new one when the header
// is missing or invalid. The resolved ID is propagated through the
// request context via [NewContext] / [FromContext].
//
// # Tracing fallback
//
// [FromContextOrTraceID] returns the request ID when available, otherwise
// falls back to the OpenTelemetry trace ID, providing a consistent
// correlation key for logs even when explicit request IDs are absent.
//
// Example:
//
//	gen := requestid.NewGenerator(
//	    requestid.WithHeaderName("X-Request-ID"),
//	    requestid.WithGenerateIfMissing(true),
//	)
//	id := gen.Extract(headers)
//	ctx := requestid.NewContext(ctx, id)
package requestid
