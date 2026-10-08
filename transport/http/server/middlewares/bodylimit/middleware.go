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
	maxSize int64
	opts    *options
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
// Error responses are written by the [WithErrorWriter] writer when one is set,
// otherwise with the responder.WriteError helper, which enables structured
// error responses via content negotiation (JSON/XML based on Accept header).
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if limit := m.limit(r); limit > 0 {
			// Declared-size pre-check: cheap up-front rejection for
			// clients that politely told us they were too big.
			if r.ContentLength > limit {
				m.writeError(w, r, ErrBodyTooLarge, http.StatusRequestEntityTooLarge)
				return
			}
			// Chunked / unknown-size strictness: with
			// WithRequireContentLength, body-bearing requests without
			// a declared Content-Length are rejected here before the
			// handler is even invoked. Without this, chunked uploads
			// bypass the pre-check and a handler that never reads the
			// body lets the attacker hold the connection until
			// ReadTimeout — turning the size cap into a soft bound.
			if m.opts.requireContentLength && r.ContentLength < 0 {
				if bodyMethods.Contains(r.Method) {
					m.writeError(w, r, ErrLengthRequired, http.StatusLengthRequired)
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

// limit returns the body limit for r: the [LimitFunc] result when one is
// set and returns non-zero, the static limit otherwise.
func (m *middleware) limit(r *http.Request) int64 {
	if m.opts.limitFunc != nil {
		if limit := m.opts.limitFunc(r); limit != 0 {
			return limit
		}
	}
	return m.maxSize
}

// writeError writes a rejection with the configured error writer, or
// [responder.WriteError] when none is set.
func (m *middleware) writeError(w http.ResponseWriter, r *http.Request, err error, status int) {
	if m.opts.errorWriter != nil {
		_ = m.opts.errorWriter.WriteError(w, r, err, status) //nolint:errcheck // Best effort
		return
	}
	_ = responder.WriteError(w, r, err, status) //nolint:errcheck // Best effort
}

// New creates a new body limit middleware with the given maximum request body size.
// A maxSize of zero or less leaves requests unlimited unless a [LimitFunc]
// sets a limit.
func New(maxSize int64, opts ...Option) *middleware {
	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddleware(middlewareName, nil),
		maxSize:        maxSize,
		opts:           newOptions(opts...),
	}
}

// Middleware returns an HTTP middleware that limits the request body size.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func Middleware(maxSize int64, opts ...Option) func(http.Handler) http.Handler {
	return New(maxSize, opts...).Handler
}
