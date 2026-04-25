// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package headers centralizes HTTP and gRPC header name constants and
// provides adapter types that unify header access across transports.
//
// Header name strings are interned at package init to avoid per-request
// allocations when the same constant is used across many goroutines.
// Raw (non-interned) constants are also available for cases that need
// the original string value.
//
// [HTTPHeaderGetter] and [GRPCHeaderGetter] adapt [net/http.Request] and
// [google.golang.org/grpc/metadata] respectively to a common interface
// consumed by packages like [clientip] and [requestid].
//
// Example:
//
//	adapter := headers.NewHTTPHeaderGetter(request)
//	values := adapter.GetHeader(headers.HeaderContentType)
package headers
