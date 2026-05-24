// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

const middlewareName = "auth"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// contextKey is the type used for storing auth data in context.
type contextKey struct{}

var authContextKey = contextKey{}

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	opts *options
}

// Dependencies returns optional middlewares that should run before auth.
func (m *middleware) Dependencies() []string {
	return nil
}

// New creates a new authentication middleware with the given options.
func New(opt ...Option) *middleware {
	opts := newOptions(opt...)
	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		opts: opts,
	}
}

// Middleware returns an HTTP middleware function that authenticates requests.
// Prefer [New] when you need the full [middlewares.Middleware] surface
// (e.g. to add the middleware to a dependency-ordered chain).
func Middleware(opt ...Option) func(http.Handler) http.Handler {
	return New(opt...).Handler
}

// Handler wraps an http.Handler with authentication.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := m.InternPath(r.URL.Path)

		if m.ShouldIgnore(path) {
			m.LogIgnored(r.Context(), path)
			next.ServeHTTP(w, r)
			return
		}

		data, err := m.authenticate(r, path)
		if err != nil {
			m.handleAuthError(w, r, err)
			return
		}

		ctx := context.WithValue(r.Context(), authContextKey, data)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authenticate runs token extraction and validation and returns the
// associated principal data on success. The caller is responsible for
// installing the result into the request context — keeping context
// derivation in Handler makes the inheritance visible to static analysis
// (contextcheck) and avoids round-tripping a context through the return.
func (m *middleware) authenticate(r *http.Request, path string) (any, error) {
	ctx := r.Context()

	token, err := m.opts.tokenExtractor.ExtractToken(r)
	if err != nil {
		m.LogDebug(ctx, "token extraction failed", path,
			slog.String("method", r.Method),
			slog.String("error", err.Error()))
		return nil, err
	}

	if token == "" {
		m.LogDebug(ctx, "missing token", path,
			slog.String("method", r.Method))
		return nil, ErrMissingToken
	}

	if m.opts.authFunc == nil {
		return nil, ErrUnauthorized
	}

	data, err := m.opts.authFunc.Authenticate(ctx, token)
	if err != nil {
		m.LogDebug(ctx, "authentication failed", path,
			slog.String("method", r.Method),
			slog.String("error", err.Error()))
		return nil, err
	}

	m.LogDebug(ctx, "authentication successful", path,
		slog.String("method", r.Method))

	return data, nil
}

func (m *middleware) handleAuthError(w http.ResponseWriter, r *http.Request, err error) {
	if m.opts.errorHandler != nil {
		m.opts.errorHandler.HandleError(w, r, err)
		return
	}

	if errors.Is(err, ErrMissingToken) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="Restricted"`)
		http.Error(w, "Missing or invalid authorization", http.StatusUnauthorized)
		return
	}
	http.Error(w, "Authentication failed", http.StatusUnauthorized)
}

// FromContext extracts authentication data from the context.
// Returns nil if no authentication data is present.
func FromContext(ctx context.Context) any {
	return ctx.Value(authContextKey)
}

// MustFromContext extracts authentication data from the context.
// Panics if no authentication data is present.
func MustFromContext(ctx context.Context) any {
	data := FromContext(ctx)
	if data == nil {
		panic("auth: no authentication data in context")
	}
	return data
}

// ExtractBearerToken creates a TokenExtractor that extracts Bearer tokens
// from the Authorization header. This is the default token extractor.
func ExtractBearerToken() TokenExtractor {
	return ExtractTokenFromHeader("Authorization", func(value string) (string, error) {
		const bearerPrefix = "bearer "
		if corestrings.TimingSafePrefixMatch(value, bearerPrefix) {
			return strings.TrimSpace(value[len(bearerPrefix):]), nil
		}
		return "", ErrInvalidToken
	})
}

// ExtractTokenFromHeader creates a TokenExtractor that extracts tokens
// from a specified HTTP header with custom validation logic.
func ExtractTokenFromHeader(header string, fn func(value string) (string, error)) TokenExtractor {
	return TokenExtractorFunc(func(r *http.Request) (string, error) {
		value := r.Header.Get(header)
		if value == "" {
			return "", ErrMissingToken
		}

		value = strings.TrimSpace(value)
		token, err := fn(value)
		if err != nil {
			return "", err
		}

		if token == "" {
			return "", ErrMissingToken
		}

		return token, nil
	})
}
