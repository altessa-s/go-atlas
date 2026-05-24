// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import (
	"context"
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/auth/static"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
)

// Re-exports from [github.com/altessa-s/go-atlas/auth/static] so callers can
// configure a store and reach the HTTP integration with a single import.
type (
	// TokenStore validates a token and returns associated data.
	TokenStore = static.TokenStore

	// InMemoryStore is the default in-memory [TokenStore].
	InMemoryStore = static.InMemoryStore

	// Option configures an [InMemoryStore].
	Option = static.Option

	// Metrics records validation telemetry.
	Metrics = static.Metrics

	// RateLimiter gates validation attempts.
	RateLimiter = static.RateLimiter

	// RateLimitedStore wraps a [TokenStore] with a [RateLimiter].
	RateLimitedStore = static.RateLimitedStore

	// KeyFunc derives the rate-limit key for a request.
	KeyFunc = static.KeyFunc
)

// NewInMemoryStore constructs an [InMemoryStore]. See
// [github.com/altessa-s/go-atlas/auth/static.NewInMemoryStore].
func NewInMemoryStore(opt ...Option) *InMemoryStore { return static.NewInMemoryStore(opt...) }

// WithInitialTokens seeds the store with the given token→data pairs.
func WithInitialTokens(v map[string]any) Option { return static.WithInitialTokens(v) }

// WithMetrics attaches a [*Metrics] to the store.
func WithMetrics(v *Metrics) Option { return static.WithMetrics(v) }

// WithHMACKey overrides the per-instance random digest key.
func WithHMACKey(v []byte) Option { return static.WithHMACKey(v) }

// NewMetrics constructs a [*Metrics] bound to the given collector and subsystem.
func NewMetrics(collector metrics.Collector, subsystem string) *Metrics {
	return static.NewMetrics(collector, subsystem)
}

// NewRateLimitedStore wraps store with a [RateLimiter].
func NewRateLimitedStore(store TokenStore, limiter RateLimiter, keyFn KeyFunc) *RateLimitedStore {
	return static.NewRateLimitedStore(store, limiter, keyFn)
}

// AuthFunc returns an [auth.AuthFunc] that validates the token against store.
// Errors from store are wrapped so callers can match both the transport-level
// [auth.ErrUnauthorized] / [auth.ErrInvalidToken] sentinels and the underlying
// [static.ErrInvalidToken] / [static.ErrEmptyToken] / [static.ErrRateLimited]
// causes via [errors.Is]. The original error chain is preserved for logs and
// custom [auth.ErrorHandler] implementations.
//
// Panics if store is nil — an unauthenticated middleware is not a usable
// default.
func AuthFunc(store TokenStore) auth.AuthFunc {
	if store == nil {
		panic("transport/http/.../auth/static: AuthFunc requires a non-nil TokenStore")
	}
	return auth.AuthenticateFunc(func(ctx context.Context, token string) (any, error) {
		data, err := store.Validate(ctx, token)
		if err != nil {
			return nil, translateError(err)
		}
		return data, nil
	})
}

func translateError(err error) error {
	switch {
	case errors.Is(err, static.ErrInvalidToken), errors.Is(err, static.ErrEmptyToken):
		return fmt.Errorf("%w: %w", auth.ErrUnauthorized, err)
	case errors.Is(err, static.ErrRateLimited):
		return fmt.Errorf("%w: %w", auth.ErrUnauthorized, err)
	default:
		return fmt.Errorf("auth: %w", err)
	}
}
