// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"context"
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/internal/headers"
	"github.com/altessa-s/go-atlas/transport/internal/requestid"
	"github.com/altessa-s/go-atlas/transport/internal/validation"
)

// singleHeaderAdapter adapts HTTPHeaderGetter to requestid.HeaderGetter interface.
type singleHeaderAdapter struct {
	*headers.HTTPHeaderGetter
}

// GetHeader returns the first value for the given header name.
func (s *singleHeaderAdapter) GetHeader(name string) string {
	return s.GetSingleHeader(name)
}

const middlewareName = "requestid"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	gen *requestid.Generator
}

// Dependencies returns middlewares that requestid requires to run before it.
// Requestid has no dependencies.
func (m *middleware) Dependencies() []string { return nil }

// Handler wraps an http.Handler with request ID extraction/generation functionality.
//
// Request ID handling logic (consistent with gRPC interceptor):
//  1. If a valid UUID v4 is present in the request header, it will be used
//  2. If no header is present or the UUID is invalid:
//     - If generateIfMissing is true (default): generate a new UUID v4
//     - If generateIfMissing is false: no request ID will be set
//  3. The request ID is stored in the context and set in the response header
//
// This middleware never returns an error - invalid request IDs are simply
// ignored and a new one is generated (if generateIfMissing is true).
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Skip OPTIONS requests
		if request.Method == http.MethodOptions {
			next.ServeHTTP(writer, request)
			return
		}

		// Extract existing ID from header using HeaderGetter and generate if needed
		adapter := &singleHeaderAdapter{headers.NewHTTPHeaderGetter(request)}
		reqID := m.gen.Extract(adapter)

		// If generation is disabled and ID is missing or invalid, return 400
		if !m.gen.GenerateIfMissing() {
			raw := adapter.GetHeader(m.gen.HeaderName())
			if raw == "" || !validation.IsValidUUIDv4(raw) {
				http.Error(writer, "invalid request id", http.StatusBadRequest)
				return
			}
		}

		// If we have a request ID, add it to context and response header
		if reqID != "" {
			writer.Header().Set(m.gen.HeaderName(), reqID)
			ctx := requestid.NewContext(request.Context(), reqID)
			request = request.WithContext(ctx)
		}

		next.ServeHTTP(writer, request)
	})
}

// New creates a new request ID [Middleware] with the given generator.
// The generator controls the header name and whether a new UUID v4 is
// generated when the incoming header is missing or invalid.
func New(gen *requestid.Generator) *middleware {
	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddleware(middlewareName, nil),
		gen:            gen,
	}
}

// RequestId returns an HTTP middleware that extracts or generates a request ID.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func RequestId(gen *requestid.Generator) func(next http.Handler) http.Handler {
	return New(gen).Handler
}

// FromContext returns the request ID stored by this middleware from the context.
// Returns an empty string if no request ID is present. The value is set by the
// [middleware.Handler] method or manually via [NewContext].
func FromContext(ctx context.Context) string {
	return requestid.FromContext(ctx)
}

// NewContext returns a new context with the given request ID.
// This is useful for injecting a request ID into the context manually,
// for example in tests. Retrieve it later with [FromContext].
func NewContext(ctx context.Context, id string) context.Context {
	return requestid.NewContext(ctx, id)
}
