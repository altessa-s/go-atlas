// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/altessa-s/go-atlas-net/clientip"
	"github.com/altessa-s/go-atlas-net/httpx"
	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/mtls"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	"github.com/altessa-s/go-atlas/transport/policy/fallback"

	sharedlimiter "github.com/altessa-s/go-atlas/coord/limiters"
)

const middlewareName = "limiter"

// Name returns the middleware name used for dependency resolution and chain ordering.
func Name() string { return middlewareName }

// ID is a lightweight [middlewares.Middleware] reference for this package,
// suitable for passing to exclusion lists.
var ID = middlewares.Noop(middlewareName)

// Compile-time interface assertion.
var _ middlewares.Middleware = (*middleware)(nil)

type middleware struct {
	middlewares.BaseMiddleware
	limiter          sharedlimiter.Limiter
	fallbackBehavior fallback.Behavior
}

// Dependencies returns optional middlewares that should run before limiter.
func (m *middleware) Dependencies() []string {
	return []string{auth.Name(), mtls.Name()}
}

// RequiredDependencies returns middlewares that limiter requires to function.
func (m *middleware) RequiredDependencies() []string {
	return []string{realip.Name()}
}

// New creates a new rate limiter middleware with the given [sharedlimiter.Limiter]
// and options. The middleware declares a dependency on the "realip" middleware
// to identify clients by IP address; if realip is absent, [http.Request.RemoteAddr]
// is used as a fallback.
func New(limiter sharedlimiter.Limiter, opt ...Option) *middleware {
	opts := newOptions(opt...)

	return &middleware{
		BaseMiddleware: middlewares.NewBaseMiddlewareWithFilter(
			middlewareName,
			opts.ignorePaths,
			opts.ignorePatterns,
			opts.logger,
		),
		limiter:          limiter,
		fallbackBehavior: opts.fallbackBehavior,
	}
}

// Middleware returns an HTTP middleware that limits the rate of incoming requests.
// This is a convenience function; prefer New() for access to the full Middleware interface.
func Middleware(limiter sharedlimiter.Limiter, opt ...Option) func(http.Handler) http.Handler {
	return New(limiter, opt...).Handler
}

// Handler wraps an http.Handler with rate limiting functionality.
func (m *middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := m.rateLimit(w, r); err != nil {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *middleware) rateLimit(w http.ResponseWriter, r *http.Request) error {
	path := r.URL.Path
	ctx := r.Context()

	if m.ShouldIgnoreRequest(r) {
		m.LogIgnored(ctx, path)
		return nil
	}

	if ip := clientip.FromContext(ctx); ip.IsValid() {
		ctx = sharedlimiter.ContextWithClientIP(ctx, ip.String())
	}

	if token := extractBearerToken(r); token != "" {
		ctx = sharedlimiter.ContextWithAuthToken(ctx, token)
	}

	info, err := m.limiter.Limit(ctx)

	if info != nil {
		m.LogDebug(ctx, "setting rate limit headers", path,
			slog.String("method", r.Method),
			slog.Int64("limit", info.Limit),
			slog.Int64("remaining", info.Remaining),
			slog.Int64("reset", info.Reset))

		w.Header().Set(httpx.RateLimitLimit, strconv.FormatInt(info.Limit, 10))
		w.Header().Set(httpx.RateLimitRemaining, strconv.FormatInt(info.Remaining, 10))
		w.Header().Set(httpx.RateLimitReset, strconv.FormatInt(info.Reset, 10))
	}

	if err != nil {
		if errors.Is(err, sharedlimiter.ErrLimitExceeded) {
			m.LogDebug(ctx, "rate limit exceeded", path,
				slog.String("method", r.Method))
			if info != nil {
				w.Header().Set(httpx.RetryAfter, strconv.FormatInt(retryAfterSeconds(info.Reset, time.Now()), 10))
			}
			http.Error(w, "Rate Limit Exceeded", http.StatusTooManyRequests)
			return err
		}

		m.LogWarn(ctx, "rate limit check failed", path, nil, slog.String("classification", diagnostics.ErrorClass(err)),
			slog.String("method", r.Method))

		switch m.fallbackBehavior {
		case fallback.Allow:
			return nil
		case fallback.Deny:
			http.Error(w, "Service Temporarily Unavailable", http.StatusServiceUnavailable)
			return err
		default:
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return err
		}
	}

	m.LogDebug(ctx, "rate limit check passed", path,
		slog.String("method", r.Method))
	return nil
}

func extractBearerToken(r *http.Request) string {
	token, _ := httpx.ParseBearer(r.Header.Values("Authorization"))
	return token
}

// retryAfterSeconds converts a Unix reset time into the delay-seconds form
// Retry-After requires (RFC 9110 §10.2.3). Storages round the reset down to
// whole seconds, so one second is added to keep a client from retrying just
// before the window frees a slot.
func retryAfterSeconds(reset int64, now time.Time) int64 {
	return max(reset-now.Unix()+1, 1)
}
