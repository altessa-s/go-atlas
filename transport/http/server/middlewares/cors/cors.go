// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cors

import (
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Header names for CORS.
const (
	HeaderOrigin                         = "Origin"
	HeaderAccessControlRequestMethod     = "Access-Control-Request-Method"
	HeaderAccessControlRequestHeaders    = "Access-Control-Request-Headers"
	HeaderAccessControlAllowOrigin       = "Access-Control-Allow-Origin"
	HeaderAccessControlAllowMethods      = "Access-Control-Allow-Methods"
	HeaderAccessControlAllowHeaders      = "Access-Control-Allow-Headers"
	HeaderAccessControlAllowCredentials  = "Access-Control-Allow-Credentials"
	HeaderAccessControlExposeHeaders     = "Access-Control-Expose-Headers"
	HeaderAccessControlMaxAge            = "Access-Control-Max-Age"
	HeaderAccessControlAllowPrivateNet   = "Access-Control-Allow-Private-Network"
	HeaderAccessControlRequestPrivateNet = "Access-Control-Request-Private-Network"
	HeaderVary                           = "Vary"
)

const middlewareName = "cors"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

// middleware is the CORS middleware implementation.
type middleware struct {
	middlewares.BaseMiddleware
	opts *options

	// Pre-computed values for performance
	allowedOriginsMap map[string]struct{}
	allowedMethodsMap map[string]struct{}
	allowedMethodsStr string
	allowedHeadersStr string
	allowedHeadersMap map[string]struct{}
	exposedHeadersStr string
	maxAgeStr         string
}

// Dependencies returns middlewares that cors requires to run before it.
// CORS has no dependencies.
func (m *middleware) Dependencies() []string {
	return nil
}

// New creates a new CORS middleware with the given options.
//
// Example:
//
//	mw := cors.New(cors.WithAllowedOrigins("https://example.com"))
//	handler := mw.Handler(yourHandler)
func New(opt ...Option) *middleware {
	opts := newOptions(opt...)

	if opts.allowAllOrigins && opts.allowCredentials {
		panic("cors: insecure configuration: AllowAllOrigins and AllowCredentials cannot both be enabled " +
			"— this allows any website to make credentialed requests on behalf of your users")
	}

	// A regex that matches arbitrary foreign origins is equivalent to AllowAllOrigins:
	// combined with credentials it lets any site issue credentialed requests and read
	// the response (the handler echoes the request Origin, not a wildcard). Reject the
	// combination the same way. Narrow, app-specific patterns are unaffected.
	if opts.allowCredentials {
		if p := firstBroadOriginPattern(opts.allowedOriginPatterns); p != nil {
			panic("cors: insecure configuration: AllowCredentials with an origin pattern that matches arbitrary origins (`" +
				p.String() + "`) — this allows any website to make credentialed requests on behalf of your users; " +
				"anchor the pattern to your own domains")
		}
	}

	m := &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		opts: opts,
	}

	// Pre-compute values for performance
	m.precompute()

	return m
}

// Middleware returns a middleware handler function.
// This is a convenience function; prefer New() for access to the full Middleware interface.
//
// Example:
//
//	handler := cors.Middleware(cors.WithAllowedOrigins("https://example.com"))(yourHandler)
func Middleware(opt ...Option) func(next http.Handler) http.Handler {
	m := New(opt...)
	return m.Handler
}

// Handler wraps an http.Handler with CORS support.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this path should be ignored
		if m.ShouldIgnore(r.URL.Path) {
			m.LogIgnored(r.Context(), r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get(HeaderOrigin)

		// If no Origin header, this is not a CORS request
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if origin is allowed
		if !m.isOriginAllowed(origin) {
			m.LogDebug(r.Context(), "origin not allowed", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		// Handle preflight OPTIONS request
		if r.Method == http.MethodOptions {
			m.handlePreflight(w, r, origin)
			if !m.opts.optionsPassthrough {
				w.WriteHeader(m.opts.optionsSuccessStatus)
				return
			}
		} else {
			// Handle actual request
			m.handleActual(w, origin)
		}

		next.ServeHTTP(w, r)
	})
}

// precompute calculates values that can be reused across requests.
func (m *middleware) precompute() {
	// Build allowed origins map for O(1) lookup
	m.allowedOriginsMap = make(map[string]struct{}, len(m.opts.allowedOrigins))
	for _, origin := range m.opts.allowedOrigins {
		m.allowedOriginsMap[origin] = struct{}{}
	}

	// Pre-join methods
	m.allowedMethodsStr = strings.Join(m.opts.allowedMethods, ", ")
	m.allowedMethodsMap = make(map[string]struct{}, len(m.opts.allowedMethods))
	for _, method := range m.opts.allowedMethods {
		m.allowedMethodsMap[corestrings.InternUpperString(method)] = struct{}{}
	}

	// Pre-join and normalize headers
	normalizedHeaders := make([]string, len(m.opts.allowedHeaders))
	m.allowedHeadersMap = make(map[string]struct{}, len(m.opts.allowedHeaders))
	for i, h := range m.opts.allowedHeaders {
		normalized := http.CanonicalHeaderKey(h)
		normalizedHeaders[i] = normalized
		m.allowedHeadersMap[strings.ToLower(h)] = struct{}{}
	}
	m.allowedHeadersStr = strings.Join(normalizedHeaders, ", ")

	// Pre-join exposed headers
	if len(m.opts.exposedHeaders) > 0 {
		m.exposedHeadersStr = strings.Join(m.opts.exposedHeaders, ", ")
	}

	// Pre-convert max age
	if m.opts.maxAge > 0 {
		m.maxAgeStr = strconv.Itoa(m.opts.maxAge)
	}
}

// broadOriginCanaries are origins that no domain-anchored pattern should match.
// A pattern matching any of them is effectively allow-all (e.g. `.*`, `^https?://`)
// and is unsafe to combine with credentials.
var broadOriginCanaries = []string{
	"https://cors-wildcard-canary.invalid",
	"http://cors-wildcard-canary.invalid",
	"null",
}

// firstBroadOriginPattern returns the first pattern that matches a canary foreign
// origin, or nil when every pattern is suitably narrow. Returning the regexp (not
// its string) avoids conflating "not found" with an empty-string pattern, which
// itself matches everything.
func firstBroadOriginPattern(patterns []*regexp.Regexp) *regexp.Regexp {
	for _, pattern := range patterns {
		if pattern == nil {
			continue
		}
		if slices.ContainsFunc(broadOriginCanaries, pattern.MatchString) {
			return pattern
		}
	}
	return nil
}

// isOriginAllowed checks if the given origin is allowed.
func (m *middleware) isOriginAllowed(origin string) bool {
	// Allow all origins
	if m.opts.allowAllOrigins {
		return true
	}

	// Check exact match
	if _, ok := m.allowedOriginsMap[origin]; ok {
		return true
	}

	// Check patterns
	for _, pattern := range m.opts.allowedOriginPatterns {
		if pattern.MatchString(origin) {
			return true
		}
	}

	return false
}

// handlePreflight handles preflight OPTIONS requests.
func (m *middleware) handlePreflight(w http.ResponseWriter, r *http.Request, origin string) {
	h := w.Header()

	// Always add Vary headers for preflight
	h.Add(HeaderVary, HeaderOrigin)
	h.Add(HeaderVary, HeaderAccessControlRequestMethod)
	h.Add(HeaderVary, HeaderAccessControlRequestHeaders)

	// Validate request method
	reqMethod := r.Header.Get(HeaderAccessControlRequestMethod)
	if reqMethod == "" {
		m.LogDebug(r.Context(), "missing Access-Control-Request-Method", r.URL.Path)
		return
	}

	if !m.isMethodAllowed(reqMethod) {
		m.LogDebug(r.Context(), "method not allowed", r.URL.Path)
		return
	}

	// Validate request headers
	reqHeaders := r.Header.Get(HeaderAccessControlRequestHeaders)
	if reqHeaders != "" && !m.areHeadersAllowed(reqHeaders) {
		m.LogDebug(r.Context(), "headers not allowed", r.URL.Path)
		return
	}

	// Set origin header
	m.setOriginHeader(h, origin)

	// Set allowed methods
	h.Set(HeaderAccessControlAllowMethods, m.allowedMethodsStr)

	// Set allowed headers
	if m.allowedHeadersStr != "" {
		h.Set(HeaderAccessControlAllowHeaders, m.allowedHeadersStr)
	}

	// Set credentials
	if m.opts.allowCredentials {
		h.Set(HeaderAccessControlAllowCredentials, "true")
	}

	// Set max age
	if m.maxAgeStr != "" {
		h.Set(HeaderAccessControlMaxAge, m.maxAgeStr)
	}

	// Handle Private Network Access
	if m.opts.allowPrivateNetwork && r.Header.Get(HeaderAccessControlRequestPrivateNet) == "true" {
		h.Set(HeaderAccessControlAllowPrivateNet, "true")
	}
}

// handleActual handles actual (non-preflight) CORS requests.
func (m *middleware) handleActual(w http.ResponseWriter, origin string) {
	h := w.Header()

	// Add Vary header
	h.Add(HeaderVary, HeaderOrigin)

	// Set origin header
	m.setOriginHeader(h, origin)

	// Set credentials
	if m.opts.allowCredentials {
		h.Set(HeaderAccessControlAllowCredentials, "true")
	}

	// Set exposed headers
	if m.exposedHeadersStr != "" {
		h.Set(HeaderAccessControlExposeHeaders, m.exposedHeadersStr)
	}
}

// setOriginHeader sets the Access-Control-Allow-Origin header.
func (m *middleware) setOriginHeader(h http.Header, origin string) {
	if m.opts.allowAllOrigins && !m.opts.allowCredentials {
		// Use wildcard when all origins are allowed and no credentials
		h.Set(HeaderAccessControlAllowOrigin, "*")
	} else {
		// Echo the specific origin
		h.Set(HeaderAccessControlAllowOrigin, origin)
	}
}

// isMethodAllowed checks if the given method is in the allowed list.
func (m *middleware) isMethodAllowed(method string) bool {
	_, ok := m.allowedMethodsMap[corestrings.InternUpperString(method)]
	return ok
}

// areHeadersAllowed checks if all requested headers are allowed.
func (m *middleware) areHeadersAllowed(requestedHeaders string) bool {
	// If no specific headers configured, allow all
	if len(m.allowedHeadersMap) == 0 {
		return true
	}

	headers := strings.Split(requestedHeaders, ",")
	for _, header := range headers {
		header = strings.TrimSpace(header)
		header = corestrings.InternLowerString(header)
		if _, ok := m.allowedHeadersMap[header]; !ok {
			return false
		}
	}
	return true
}
