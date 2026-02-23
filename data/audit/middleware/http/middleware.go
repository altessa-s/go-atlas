// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package http provides HTTP middleware for automatic request auditing.
package http

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/observability/tracing"
)

// Middleware returns an HTTP middleware that audits requests using the provided Auditor.
func Middleware(auditor *audit.Auditor, opts ...Option) func(next http.Handler) http.Handler {
	if auditor == nil {
		return func(next http.Handler) http.Handler { return next }
	}

	o := newOptions(opts...)

	ignorePathSet := make(map[string]struct{}, len(o.ignorePaths))
	for _, p := range o.ignorePaths {
		ignorePathSet[p] = struct{}{}
	}

	ignoreMethodsMap := make(map[string]struct{}, len(o.ignoreMethods))
	for _, m := range o.ignoreMethods {
		ignoreMethodsMap[strings.ToUpper(m)] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ignored := ignorePathSet[r.URL.Path]; ignored {
				next.ServeHTTP(w, r)
				return
			}
			for _, p := range o.ignorePatterns {
				if p.MatchString(r.URL.Path) {
					next.ServeHTTP(w, r)
					return
				}
			}
			if len(ignoreMethodsMap) > 0 {
				if _, ignored := ignoreMethodsMap[strings.ToUpper(r.Method)]; ignored {
					next.ServeHTTP(w, r)
					return
				}
			}

			start := time.Now()

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			duration := time.Since(start)

			var actor audit.Actor
			if o.actorExtractor != nil {
				actor = o.actorExtractor(r)
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

			result := audit.Result{Status: audit.ResultStatusSuccess, Code: sw.status}
			if sw.status >= http.StatusBadRequest {
				result.Status = audit.ResultStatusFailure
			}
			if sw.status >= http.StatusInternalServerError {
				result.Status = audit.ResultStatusError
			}

			// Extract context from request for correlation.
			ctx := r.Context()
			eventCtx := audit.EventContext{
				TraceID: tracing.TraceIDFromContext(ctx),
				SpanID:  tracing.SpanIDFromContext(ctx),
			}
			if o.requestIDExtractor != nil {
				eventCtx.RequestID = o.requestIDExtractor(ctx)
			}

			event := &audit.Event{
				Type:   audit.EventTypeAPIRequest,
				Action: httpMethodToAction(r.Method),
				Actor:  actor,
				Resource: audit.Resource{
					Type: "http.endpoint",
					Path: r.URL.Path,
				},
				Result:    result,
				Context:   eventCtx,
				Duration:  duration,
				Timestamp: start,
			}

			auditor.Emit(event)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if sw.wroteHeader {
		return
	}
	sw.status = code
	sw.wroteHeader = true
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	if !sw.wroteHeader {
		sw.WriteHeader(http.StatusOK)
	}
	return sw.ResponseWriter.Write(b)
}

func httpMethodToAction(method string) audit.Action {
	switch method {
	case http.MethodPost:
		return audit.ActionCreate
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return audit.ActionRead
	case http.MethodPut, http.MethodPatch:
		return audit.ActionUpdate
	case http.MethodDelete:
		return audit.ActionDelete
	default:
		return audit.ActionExecute
	}
}
