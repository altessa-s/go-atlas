// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestid

import (
	"context"
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/internal/requestidcapture"
	"github.com/altessa-s/go-atlas/transport/requestid"
)

// headerValues contains the values of the generator's configured HTTP header.
type headerValues []string

func (v headerValues) GetHeader(_ string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
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
//     - If generateIfMissing is false: the request is rejected with 400 Bad Request
//  3. The request ID is stored in the context and set in the response header
func (m *middleware) Handler(next http.Handler) http.Handler {
	// OPTIONS bypasses ID processing, even when no generator was supplied.
	var header string
	if m.gen != nil {
		header = http.CanonicalHeaderKey(m.gen.HeaderName())
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Skip OPTIONS requests
		if request.Method == http.MethodOptions {
			next.ServeHTTP(writer, request)
			return
		}

		// Extract validates the first header value and returns an empty ID when
		// generation is disabled and the client supplied no valid ID.
		reqID := m.gen.Extract(headerValues(request.Header[header]))
		if reqID == "" && !m.gen.GenerateIfMissing() {
			http.Error(writer, "invalid request id", http.StatusBadRequest)
			return
		}
		if reqID != "" {
			ctx := request.Context()
			requestidcapture.Record(ctx, reqID)
			writer.Header()[header] = []string{reqID}
			request = request.WithContext(requestid.NewContext(ctx, reqID))
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
