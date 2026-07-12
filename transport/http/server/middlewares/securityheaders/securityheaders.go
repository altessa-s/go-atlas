// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
)

// Header names as constants for consistency.
// See the package-level documentation for the OWASP references.
const (
	HeaderXContentTypeOptions    = "X-Content-Type-Options"
	HeaderXFrameOptions          = "X-Frame-Options"
	HeaderReferrerPolicy         = "Referrer-Policy"
	HeaderXXSSProtection         = "X-XSS-Protection"
	HeaderStrictTransportSec     = "Strict-Transport-Security"
	HeaderContentSecurityPolicy  = "Content-Security-Policy"
	HeaderPermissionsPolicy      = "Permissions-Policy"
	HeaderCrossOriginOpenerPol   = "Cross-Origin-Opener-Policy"
	HeaderCrossOriginEmbedderPol = "Cross-Origin-Embedder-Policy"
	HeaderCrossOriginResourcePol = "Cross-Origin-Resource-Policy"
)

const middlewareName = "securityheaders"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

// middleware is the security headers middleware implementation.
type middleware struct {
	middlewares.BaseMiddleware

	// headers is the full set of configured header name/value pairs, computed
	// once at construction so the per-request path is a single loop.
	headers []headerValue
}

// headerValue is one precomputed response header.
type headerValue struct {
	name  string
	value string
}

// Dependencies returns middlewares that securityheaders requires to run before it.
// Securityheaders has no dependencies.
func (m *middleware) Dependencies() []string {
	return nil
}

// New creates a new security headers [Middleware] with the given [Option] values.
// All header values are computed at creation time for efficient per-request
// application.
//
// Example:
//
//	mw := securityheaders.New()
//	handler := mw.Handler(yourHandler)
//
// Or using the functional form:
//
//	handler := securityheaders.Middleware()(yourHandler)
func New(opt ...Option) *middleware {
	opts := newOptions(opt...)

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		headers: buildHeaders(opts),
	}
}

// Middleware returns a middleware handler function.
// This is a convenience function; prefer New() for access to the full Middleware interface.
//
// Example:
//
//	handler := securityheaders.Middleware()(yourHandler)
func Middleware(opt ...Option) func(next http.Handler) http.Handler {
	m := New(opt...)
	return m.Handler
}

// Handler wraps an http.Handler with security headers.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this path should be ignored
		if m.ShouldIgnore(r.URL.Path) {
			m.LogIgnored(r.Context(), r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		// Set security headers before passing to next handler
		m.setHeaders(w)

		next.ServeHTTP(w, r)
	})
}

// setHeaders applies all configured security headers to the response.
func (m *middleware) setHeaders(w http.ResponseWriter) {
	h := w.Header()
	for _, hv := range m.headers {
		h.Set(hv.name, hv.value)
	}
}

// buildHeaders computes the configured header name/value pairs once at
// construction time.
func buildHeaders(opts *options) []headerValue {
	hv := make([]headerValue, 0, 10)

	// X-Content-Type-Options: nosniff — prevents MIME type sniffing attacks.
	hv = slices.AppendIf(hv, opts.contentTypeNoSniff,
		headerValue{HeaderXContentTypeOptions, "nosniff"})
	// X-Frame-Options: DENY or SAMEORIGIN — prevents clickjacking attacks.
	hv = slices.AppendIf(hv, opts.frameOptions != "",
		headerValue{HeaderXFrameOptions, string(opts.frameOptions)})
	// Referrer-Policy — controls how much referrer information is sent.
	hv = slices.AppendIf(hv, opts.referrerPolicy != "",
		headerValue{HeaderReferrerPolicy, string(opts.referrerPolicy)})
	// X-XSS-Protection: 0 — disables the browser XSS filter (recommended, as
	// the filter can itself introduce vulnerabilities).
	hv = slices.AppendIf(hv, opts.xssProtectionDisabled,
		headerValue{HeaderXXSSProtection, "0"})
	// Strict-Transport-Security (HSTS) — enforces HTTPS connections.
	hv = slices.AppendIfFunc(hv, opts.hstsEnabled, func() []headerValue {
		return []headerValue{{HeaderStrictTransportSec, buildHSTSValue(opts)}}
	})
	// Content-Security-Policy — controls which resources can be loaded.
	hv = slices.AppendIf(hv, opts.contentSecurityPolicy != "",
		headerValue{HeaderContentSecurityPolicy, opts.contentSecurityPolicy})
	// Permissions-Policy — controls which browser features can be used.
	hv = slices.AppendIf(hv, opts.permissionsPolicy != "",
		headerValue{HeaderPermissionsPolicy, opts.permissionsPolicy})
	// Cross-Origin-Opener-Policy / -Embedder-Policy / -Resource-Policy.
	// All opt-in (unset by default): they can break cross-window integrations and
	// cross-origin subresources, so they are emitted only when configured.
	hv = slices.AppendIf(hv, opts.crossOriginOpenerPolicy != "",
		headerValue{HeaderCrossOriginOpenerPol, string(opts.crossOriginOpenerPolicy)})
	hv = slices.AppendIf(hv, opts.crossOriginEmbedderPolicy != "",
		headerValue{HeaderCrossOriginEmbedderPol, string(opts.crossOriginEmbedderPolicy)})
	hv = slices.AppendIf(hv, opts.crossOriginResourcePolicy != "",
		headerValue{HeaderCrossOriginResourcePol, string(opts.crossOriginResourcePolicy)})

	return hv
}

// buildHSTSValue constructs the Strict-Transport-Security header value.
func buildHSTSValue(opts *options) string {
	parts := make([]string, 0, 3)

	parts = append(parts, "max-age="+strconv.Itoa(opts.hstsMaxAge))

	parts = slices.AppendIf(parts, opts.hstsIncludeSubDomains, "includeSubDomains")
	parts = slices.AppendIf(parts, opts.hstsPreload, "preload")

	return strings.Join(parts, "; ")
}
