// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bodylimit

import (
	"errors"
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/responder"
)

// ErrBodyTooLarge is returned by the body limit middleware (created via [New] or
// [Middleware]) when the request Content-Length exceeds the configured limit.
// The error is written as a structured 413 response via [responder.WriteError].
var ErrBodyTooLarge = errors.New("request body too large")

const middlewareName = "bodylimit"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	maxSize int64
}

// Dependencies returns middlewares that bodylimit requires to run before it.
// Bodylimit has no dependencies.
func (m *middleware) Dependencies() []string { return nil }

// Handler wraps an http.Handler with body size limiting functionality.
// If the request Content-Length exceeds the limit, it returns 413 Request Entity Too Large.
// It also wraps the request body in http.MaxBytesReader to enforce the limit during reading.
//
// Error responses are written using the responder.WriteError helper, which enables
// structured error responses via content negotiation (JSON/XML based on Accept header).
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.maxSize > 0 {
			if r.ContentLength > m.maxSize {
				_ = responder.WriteError(w, r, ErrBodyTooLarge, http.StatusRequestEntityTooLarge) //nolint:errcheck // Best effort
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, m.maxSize)
		}
		next.ServeHTTP(w, r)
	})
}

// New creates a new body limit middleware with the given maximum request body size.
func New(maxSize int64) *middleware {
	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddleware(middlewareName, nil),
		maxSize:        maxSize,
	}
}

// Middleware returns an HTTP middleware that limits the request body size.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func Middleware(maxSize int64) func(http.Handler) http.Handler {
	return New(maxSize).Handler
}
