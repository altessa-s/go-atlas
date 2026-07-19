// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bodylimit

import (
	"errors"
	"net/http"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/responder"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
)

// ErrBodyTooLarge is returned by the body limit middleware (created via [New] or
// [Middleware]) when the request Content-Length exceeds the configured limit.
// The error is written as a structured 413 response via [responder.WriteError].
var ErrBodyTooLarge = errors.New("request body too large")

// ErrLengthRequired is returned when [WithRequireContentLength] is enabled
// and a body-bearing request (POST/PUT/PATCH/DELETE) arrives without a
// Content-Length header (e.g. Transfer-Encoding: chunked). It maps to
// 411 Length Required. The default pre-check only enforces the cap via
// the declared Content-Length; chunked uploads bypass the early
// rejection and only get bounded once the handler starts reading the
// body. Operators who want a hard up-front bound on body size opt in
// to this option.
var ErrLengthRequired = errors.New("request must declare Content-Length")

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
	maxSize              int64
	requireContentLength bool
}

// Option configures a [middleware] instance. Use the With… helpers to
// produce values.
type Option func(*middleware)

// WithRequireContentLength rejects body-bearing requests
// (POST/PUT/PATCH/DELETE) that arrive without a declared Content-Length
// (e.g. Transfer-Encoding: chunked) with 411 Length Required. Off by
// default — chunked uploads are still accepted and the size cap is
// enforced lazily by http.MaxBytesReader once the handler reads the
// body. Turn this on when you need the cap rejected UP FRONT (e.g. a
// public endpoint where a chunked-drip attacker should not be allowed
// to keep a request goroutine alive until the full ReadTimeout).
func WithRequireContentLength() Option {
	return func(m *middleware) {
		m.requireContentLength = true
	}
}

// bodyMethods enumerates HTTP methods that ordinarily carry a request
// body. GET / HEAD / OPTIONS / TRACE / CONNECT are excluded — they may
// have a body but practically never do, and the strict pre-check would
// otherwise reject perfectly benign GET requests when paired with
// WithRequireContentLength.
var bodyMethods = coremaps.NewImmutableMap(map[string]struct{}{
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
})

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
			// Declared-size pre-check: cheap up-front rejection for
			// clients that politely told us they were too big.
			if r.ContentLength > m.maxSize {
				_ = responder.WriteError(w, r, ErrBodyTooLarge, http.StatusRequestEntityTooLarge) //nolint:errcheck // Best effort
				return
			}
			// Chunked / unknown-size strictness: with
			// WithRequireContentLength, body-bearing requests without
			// a declared Content-Length are rejected here before the
			// handler is even invoked. Without this, chunked uploads
			// bypass the pre-check and a handler that never reads the
			// body lets the attacker hold the connection until
			// ReadTimeout — turning the size cap into a soft bound.
			if m.requireContentLength && r.ContentLength < 0 {
				if bodyMethods.Contains(r.Method) {
					_ = responder.WriteError(w, r, ErrLengthRequired, http.StatusLengthRequired) //nolint:errcheck // Best effort
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, m.maxSize)
		}
		next.ServeHTTP(w, r)
	})
}

// New creates a new body limit middleware with the given maximum request body size.
func New(maxSize int64, opts ...Option) *middleware {
	m := &middleware{
		BaseMiddleware: middlewares.NewBaseMiddleware(middlewareName, nil),
		maxSize:        maxSize,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Middleware returns an HTTP middleware that limits the request body size.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func Middleware(maxSize int64, opts ...Option) func(http.Handler) http.Handler {
	return New(maxSize, opts...).Handler
}
