// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/altessa-s/go-atlas/data/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/realip"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/headers"

	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
)

// HTTP status code for rate limit exceeded.
const (
	StatusTooManyRequests    = http.StatusTooManyRequests
	StatusServiceUnavailable = http.StatusServiceUnavailable
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
	return nil
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
	path := m.InternPath(r.URL.Path)
	ctx := r.Context()

	if m.ShouldIgnore(path) {
		m.LogIgnored(ctx, path)
		return nil
	}

	if ip := clientip.FromContext(ctx); ip.IsValid() {
		ctx = tokenbucket.ContextWithClientIP(ctx, ip.String())
	}

	if token := extractBearerToken(r); token != "" {
		ctx = tokenbucket.ContextWithAuthToken(ctx, token)
	}

	info, err := m.limiter.Limit(ctx)

	if info != nil {
		m.LogDebug(ctx, "setting rate limit headers", path,
			slog.String("method", r.Method),
			slog.Int64("limit", info.Limit),
			slog.Int64("remaining", info.Remaining),
			slog.Int64("reset", info.Reset))

		w.Header().Set(headers.RateLimitLimit, strconv.FormatInt(info.Limit, 10))
		w.Header().Set(headers.RateLimitRemaining, strconv.FormatInt(info.Remaining, 10))
		w.Header().Set(headers.RateLimitReset, strconv.FormatInt(info.Reset, 10))
	}

	if err != nil {
		if errors.Is(err, sharedlimiter.ErrLimitExceeded) {
			m.LogDebug(ctx, "rate limit exceeded", path,
				slog.String("method", r.Method))
			if info != nil {
				w.Header().Set(headers.RetryAfter, strconv.FormatInt(info.Reset, 10))
			}
			http.Error(w, "Rate Limit Exceeded", StatusTooManyRequests)
			return err
		}

		m.LogWarn(ctx, "rate limit check failed", path, err,
			slog.String("method", r.Method))

		switch m.fallbackBehavior {
		case fallback.Allow:
			return nil
		case fallback.Deny:
			http.Error(w, "Service Temporarily Unavailable", StatusServiceUnavailable)
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
	authHeader := r.Header.Get("Authorization")
	const bearerPrefix = "Bearer "
	if len(authHeader) >= len(bearerPrefix) && strings.EqualFold(authHeader[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(authHeader[len(bearerPrefix):])
	}
	return ""
}
