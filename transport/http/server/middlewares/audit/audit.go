// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/observability/tracing"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/requestid"
)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	auditor          *audit.Auditor
	opts             *options
	ignoreMethodsMap map[string]struct{}
}

// Dependencies returns optional middlewares that audit reads from context.
func (m *middleware) Dependencies() []string {
	return []string{"requestid", "tracing"}
}

// RequiredDependencies returns middlewares that audit requires to function.
func (m *middleware) RequiredDependencies() []string {
	return []string{"realip"}
}

// Handler wraps an http.Handler with request auditing functionality.
func (m *middleware) Handler(next http.Handler) http.Handler {
	if m.auditor == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.ShouldIgnore(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		if len(m.ignoreMethodsMap) > 0 {
			if _, ignored := m.ignoreMethodsMap[strings.ToUpper(r.Method)]; ignored {
				next.ServeHTTP(w, r)
				return
			}
		}

		start := time.Now()

		sw := middlewares.NewResponseWriter(w, false)
		defer sw.Release()

		next.ServeHTTP(sw, r)

		duration := time.Since(start)

		var actor audit.Actor
		if m.opts.actorExtractor != nil {
			actor = m.opts.actorExtractor(r)
		}
		if actor.IP == "" {
			peerIP, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				peerIP = r.RemoteAddr
			}
			actor.IP = peerIP
		}
		if actor.UserAgent == "" {
			actor.UserAgent = r.UserAgent()
		}

		ctx := r.Context()
		info := audit.RequestInfo{
			Action:       audit.HTTPMethodToAction(r.Method),
			ResourceType: "http.endpoint",
			ResourcePath: r.URL.Path,
			Actor:        actor,
			Context: audit.NewEventContext(
				tracing.TraceIDFromContext(ctx),
				tracing.SpanIDFromContext(ctx),
				requestid.FromContext(ctx),
			),
			StartTime: start,
			Duration:  duration,
		}

		event := audit.BuildTransportEvent(info, func() audit.Result {
			return audit.ClassifyHTTPStatus(sw.StatusCode())
		})

		m.auditor.Emit(event)
	})
}

// New creates a new audit middleware with the given Auditor and options.
func New(auditor *audit.Auditor, opts ...Option) *middleware {
	if auditor == nil {
		return &middleware{
			BaseMiddleware: middlewares.NewBaseMiddleware("audit", nil),
		}
	}

	o := newOptions(opts...)

	ignoreMethodsMap := make(map[string]struct{}, len(o.ignoreMethods))
	for _, method := range o.ignoreMethods {
		ignoreMethodsMap[strings.ToUpper(method)] = struct{}{}
	}

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			"audit",
			o.ignorePaths,
			o.ignorePatterns,
			o.logger,
		),
		auditor:          auditor,
		opts:             o,
		ignoreMethodsMap: ignoreMethodsMap,
	}
}

// Middleware returns an HTTP middleware that audits requests using the provided Auditor.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func Middleware(auditor *audit.Auditor, opts ...Option) func(next http.Handler) http.Handler {
	return New(auditor, opts...).Handler
}
