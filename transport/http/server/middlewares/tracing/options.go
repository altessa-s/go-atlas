// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tracing

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/observability/tracing/propagation"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

// SpanNameFunc is a function that generates span names from HTTP requests.
// The default implementation ([defaultSpanName]) returns "METHOD /route-template".
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
	// recordClientDetails exports client addresses, User-Agent, Host and URL path.
	recordClientDetails bool `optval:"param"`
	// recordRawErrors exports raw panic values and stack traces.
	recordRawErrors bool `optval:"param"`
}

// defaultPropagator returns the default W3C Trace Context propagator.
func defaultPropagator() propagation.TextMapPropagator {
	return propagation.NewTraceContext()
}

// defaultSpanName returns the HTTP method and route template as the span name.
func defaultSpanName(r *http.Request) string {
	return middlewares.BoundedMethod(r.Method) + " " + router.RouteTemplate(r)
}

// Semantic convention attributes for HTTP tracing.
const (
	// HTTPMethodKey is the key for HTTP method attribute.
	HTTPMethodKey = "http.method"
	// HTTPRouteKey is the key for the matched route template.
	HTTPRouteKey = "http.route"
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
func requestAttributes(r *http.Request, recordClientDetails bool) []tracing.Attribute {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	attrs := []tracing.Attribute{
		tracing.String(HTTPMethodKey, middlewares.BoundedMethod(r.Method)),
		tracing.String(HTTPRouteKey, router.RouteTemplate(r)),
		tracing.String(HTTPSchemeKey, scheme),
	}
	if recordClientDetails {
		// Omit the query even when recording the URL.
		attrs = append(attrs,
			tracing.String(HTTPURLKey, scheme+"://"+r.Host+r.URL.Path),
			tracing.String(HTTPTargetKey, r.URL.Path),
			tracing.String(HTTPHostKey, r.Host),
			tracing.String(HTTPUserAgentKey, r.UserAgent()),
		)
		if ip := extractClientIP(r); ip != "" {
			attrs = append(attrs, tracing.String(NetPeerIPKey, ip))
		}
	}
	return attrs
}
