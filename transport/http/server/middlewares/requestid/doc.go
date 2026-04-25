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
// # Example
//
//	gen := requestid.NewGenerator(requestid.WithGenerateIfMissing())
//	handler := requestid.RequestId(gen)(yourHandler)
//
//	// In a downstream handler:
//	id := requestid.FromContext(r.Context())
package requestid
