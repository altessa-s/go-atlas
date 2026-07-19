// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package requestid provides middleware that extracts or generates a UUID v4
// request ID for every HTTP request.
//
// The request ID is read from the incoming request header (configurable,
// defaults to X-Request-Id). If the header is missing or contains an
// invalid UUID and generation is enabled, a new UUID v4 is generated.
// The ID is stored in the request context (retrievable via [FromContext])
// and echoed in the response header.
//
// OPTIONS requests are always passed through without processing.
//
// # Trust model
//
// A client-supplied value that passes strict UUID v4 validation is
// trusted as-is: it is echoed in the response header, stored in the
// context, and used as a log correlation field, so a hostile client
// chooses which UUID appears in logs and can reuse one across requests
// to spoof correlation. Values that fail validation never propagate, so
// arbitrary client bytes cannot reach logs through this header. There
// is no option to ignore a valid inbound value — strip or replace the
// header at the edge proxy when server-authoritative IDs are required.
//
// # Example
//
//	gen := requestid.NewGenerator(requestid.WithGenerateIfMissing())
//	handler := requestid.RequestId(gen)(yourHandler)
//
//	// In a downstream handler:
//	id := requestid.FromContext(r.Context())
package requestid
