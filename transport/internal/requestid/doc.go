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
// # Trust model
//
// A client-supplied value that passes strict UUID v4 validation (36
// characters, hyphen positions, version and variant nibbles, hex digits
// only) is trusted as-is: the transports echo it back to the client,
// store it in the request context, and use it as a log correlation
// field. A hostile client therefore chooses which UUID appears in logs
// and can reuse one across requests to spoof correlation. Values that
// fail validation are discarded and never propagate, so arbitrary
// client bytes cannot reach logs through this path (no log injection).
// There is no option to ignore a valid inbound value — services on
// untrusted edges that need server-authoritative IDs must strip or
// replace the header at the edge proxy.
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
