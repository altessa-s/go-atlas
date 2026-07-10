// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
)

// SpanNameFunc is a function that generates span names from HTTP requests.
// The default implementation ([defaultSpanName]) returns "METHOD /path".
// Override with [WithSpanNameFunc].
type SpanNameFunc func(r *http.Request) string

// options contains configuration for the tracing middleware.
type options struct {
	// ignorePaths sets paths to ignore for tracing.
	ignorePaths []string `optgen:"append"`
	// ignorePatterns sets compiled regex patterns for paths to ignore.
	ignorePatterns []*regexp.Regexp
	// propagator sets the trace context propagator.
	// Defaults to W3C Trace Context propagator.
	propagator propagation.TextMapPropagator `optgen:"default=defaultPropagator()"`
	// spanNameFunc sets a custom function for generating span names.
	// The function receives the request and should return the span name.
	spanNameFunc SpanNameFunc `optgen:"default=defaultSpanName"`
	// logger sets the logger for debug messages.
	logger *slog.Logger
}

// defaultPropagator returns the default W3C Trace Context propagator.
func defaultPropagator() propagation.TextMapPropagator {
	return propagation.NewTraceContext()
}

// defaultSpanName returns the HTTP method and path as the span name.
func defaultSpanName(r *http.Request) string {
	return r.Method + " " + r.URL.Path
}

// Semantic convention attributes for HTTP tracing.
const (
	// HTTPMethodKey is the key for HTTP method attribute.
	HTTPMethodKey = "http.method"
	// HTTPURLKey is the key for HTTP URL attribute.
	HTTPURLKey = "http.url"
	// HTTPTargetKey is the key for HTTP target (path) attribute.
	HTTPTargetKey = "http.target"
	// HTTPHostKey is the key for HTTP host attribute.
	HTTPHostKey = "http.host"
	// HTTPSchemeKey is the key for HTTP scheme attribute.
	HTTPSchemeKey = "http.scheme"
	// HTTPStatusCodeKey is the key for HTTP status code attribute.
	HTTPStatusCodeKey = "http.status_code"
	// HTTPUserAgentKey is the key for HTTP user agent attribute.
	HTTPUserAgentKey = "http.user_agent"
	// NetPeerIPKey is the key for peer IP address.
	NetPeerIPKey = "net.peer.ip"
)

// requestAttributes returns common attributes for HTTP spans.
func requestAttributes(r *http.Request) []tracing.Attribute {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	return []tracing.Attribute{
		tracing.String(HTTPMethodKey, r.Method),
		// Deliberately omit the query string: it can carry tokens, API keys, or
		// OAuth codes that would otherwise be exported verbatim to the tracing backend.
		tracing.String(HTTPURLKey, scheme+"://"+r.Host+r.URL.Path),
		tracing.String(HTTPTargetKey, r.URL.Path),
		tracing.String(HTTPHostKey, r.Host),
		tracing.String(HTTPSchemeKey, scheme),
		tracing.String(HTTPUserAgentKey, r.UserAgent()),
	}
}
