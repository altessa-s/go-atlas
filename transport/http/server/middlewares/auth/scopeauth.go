// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"net/http"
	"time"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/auth/scope"
)

// ScopeMiddleware enforces a [scope.Enforcer] over authenticated HTTP requests.
// The verified principal of type P is read from the request context via
// [FromContext] (installed by the authentication [Middleware]); keyFunc maps the
// request to the action key registered in the enforcer's registry — typically
// the matched route pattern, e.g.:
//
//	keyFunc := func(r *http.Request) string { return r.Method + " " + r.Pattern }
//
// A request whose context carries no principal of type P, or one the enforcer
// denies, is answered with 403 Forbidden and the chain stops. ScopeMiddleware
// must run after the authentication [Middleware] that populates the context.
// Pass [WithScopeAudit] to record every decision through an
// [github.com/altessa-s/go-atlas/auth/audit.Recorder].
func ScopeMiddleware[P any](e *scope.Enforcer[P], keyFunc func(*http.Request) string, opts ...ScopeMiddlewareOption[P]) func(http.Handler) http.Handler {
	var cfg scopeMiddlewareConfig[P]
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			p, ok := FromContext(r.Context()).(P)
			if !ok {
				_ = cfg.record(r, key, "", false, "principal_type_mismatch")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if err := e.Enforce(p, key); err != nil {
				_ = cfg.record(r, key, cfg.subject(p), false, "scope_denied")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			if err := cfg.record(r, key, cfg.subject(p), true, ""); err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ScopeMiddlewareOption configures [ScopeMiddleware].
type ScopeMiddlewareOption[P any] func(*scopeMiddlewareConfig[P])

type scopeMiddlewareConfig[P any] struct {
	recorder  *audit.Recorder
	subjectOf func(P) string
}

// WithScopeAudit records every authorization decision through rec, keyed on
// keyFunc's action key. subjectOf extracts the principal identity for the
// record; pass nil to leave the subject empty (it is not called when no
// principal of type P is present). When rec is configured with
// audit.FailureRequired and recording an otherwise-allowed request fails, the
// request is answered with 500 so nothing proceeds unrecorded.
func WithScopeAudit[P any](rec *audit.Recorder, subjectOf func(P) string) ScopeMiddlewareOption[P] {
	return func(c *scopeMiddlewareConfig[P]) {
		c.recorder = rec
		c.subjectOf = subjectOf
	}
}

func (c scopeMiddlewareConfig[P]) subject(p P) string {
	if c.subjectOf == nil {
		return ""
	}
	return c.subjectOf(p)
}

func (c scopeMiddlewareConfig[P]) record(r *http.Request, key, subject string, allowed bool, reason string) error {
	if c.recorder == nil {
		return nil
	}
	return c.recorder.Record(r.Context(), audit.Decision{
		Time:       time.Now().UTC(),
		Allowed:    allowed,
		Subject:    subject,
		Action:     key,
		Reason:     reason,
		Attributes: map[string]string{"transport": "http"},
	})
}
