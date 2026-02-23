// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

import (
	"fmt"
	"net/http"
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
)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

// middleware is the security headers middleware implementation.
type middleware struct {
	middlewares.BaseMiddleware
	opts *options

	// Pre-computed header values for performance
	hstsValue string
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

	m := &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			"securityheaders",
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		opts: opts,
	}

	// Pre-compute HSTS header value if enabled
	if opts.hstsEnabled {
		m.hstsValue = m.buildHSTSValue()
	}

	return m
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

	// X-Content-Type-Options: nosniff
	// Prevents MIME type sniffing attacks
	if m.opts.contentTypeNoSniff {
		h.Set(HeaderXContentTypeOptions, "nosniff")
	}

	// X-Frame-Options: DENY or SAMEORIGIN
	// Prevents clickjacking attacks
	if m.opts.frameOptions != "" {
		h.Set(HeaderXFrameOptions, string(m.opts.frameOptions))
	}

	// Referrer-Policy
	// Controls how much referrer information is sent
	if m.opts.referrerPolicy != "" {
		h.Set(HeaderReferrerPolicy, string(m.opts.referrerPolicy))
	}

	// X-XSS-Protection: 0
	// Disables browser XSS filter (recommended as it can introduce vulnerabilities)
	if m.opts.xssProtectionDisabled {
		h.Set(HeaderXXSSProtection, "0")
	}

	// Strict-Transport-Security (HSTS)
	// Enforces HTTPS connections
	if m.opts.hstsEnabled && m.hstsValue != "" {
		h.Set(HeaderStrictTransportSec, m.hstsValue)
	}

	// Content-Security-Policy
	// Controls which resources can be loaded
	if m.opts.contentSecurityPolicy != "" {
		h.Set(HeaderContentSecurityPolicy, m.opts.contentSecurityPolicy)
	}

	// Permissions-Policy
	// Controls which browser features can be used
	if m.opts.permissionsPolicy != "" {
		h.Set(HeaderPermissionsPolicy, m.opts.permissionsPolicy)
	}
}

// buildHSTSValue constructs the Strict-Transport-Security header value.
func (m *middleware) buildHSTSValue() string {
	var parts []string

	parts = append(parts, fmt.Sprintf("max-age=%d", m.opts.hstsMaxAge))

	parts = slices.AppendIf(parts, m.opts.hstsIncludeSubDomains, "includeSubDomains")
	parts = slices.AppendIf(parts, m.opts.hstsPreload, "preload")

	return strings.Join(parts, "; ")
}
