// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package headers

import (
	"context"
	"net/http"

	"google.golang.org/grpc/metadata"
)

// HTTPHeaderGetter adapts an [http.Request] to the header getter interfaces
// used by [clientip.Extractor] and [requestid.Generator]. It is safe for
// concurrent reads when the underlying request is not mutated.
type HTTPHeaderGetter struct {
	request *http.Request
}

// NewHTTPHeaderGetter creates a new HTTPHeaderGetter from an http.Request.
// Returns nil if the request is nil.
//
// Example:
//
//	adapter := headers.NewHTTPHeaderGetter(request)
//	contentType := adapter.GetSingleHeader("Content-Type")
func NewHTTPHeaderGetter(r *http.Request) *HTTPHeaderGetter {
	if r == nil {
		return nil
	}
	return &HTTPHeaderGetter{request: r}
}

// GetHeader returns all values for the given header name.
// This method is compatible with interfaces that expect []string.
//
// Example:
//
//	values := adapter.GetHeader("X-Forwarded-For")
func (h *HTTPHeaderGetter) GetHeader(name string) []string {
	if h == nil || h.request == nil {
		return nil
	}
	return h.request.Header.Values(name)
}

// GetSingleHeader returns the first value for the given header name.
// This method is compatible with interfaces that expect string.
//
// Example:
//
//	requestID := adapter.GetSingleHeader("X-Request-ID")
func (h *HTTPHeaderGetter) GetSingleHeader(name string) string {
	if h == nil || h.request == nil {
		return ""
	}
	return h.request.Header.Get(name)
}

// GRPCHeaderGetter reads incoming gRPC metadata from a [context.Context],
// adapting it to the same header getter interfaces used by HTTP adapters.
// Header names are automatically lowercased by gRPC metadata conventions.
type GRPCHeaderGetter struct {
	ctx context.Context
}

// NewGRPCHeaderGetter creates a new GRPCHeaderGetter from a context.
//
// Example:
//
//	adapter := headers.NewGRPCHeaderGetter(ctx)
//	forwardedFor := adapter.GetHeader("x-forwarded-for")
func NewGRPCHeaderGetter(ctx context.Context) *GRPCHeaderGetter {
	return &GRPCHeaderGetter{ctx: ctx}
}

// GetHeader returns all values for the given header name from gRPC metadata.
// This method is compatible with interfaces that expect []string.
//
// Example:
//
//	values := adapter.GetHeader("x-forwarded-for")
func (g *GRPCHeaderGetter) GetHeader(name string) []string {
	if g == nil || g.ctx == nil {
		return nil
	}
	return metadata.ValueFromIncomingContext(g.ctx, name)
}

// GetSingleHeader returns the first value for the given header name from gRPC metadata.
// This method is compatible with interfaces that expect string.
//
// Example:
//
//	requestID := adapter.GetSingleHeader("x-request-id")
func (g *GRPCHeaderGetter) GetSingleHeader(name string) string {
	if g == nil || g.ctx == nil {
		return ""
	}
	if vals := metadata.ValueFromIncomingContext(g.ctx, name); len(vals) > 0 {
		return vals[0]
	}
	return ""
}
