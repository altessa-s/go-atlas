// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas/auth/static"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Re-exports from [github.com/altessa-s/go-atlas/auth/static] so callers can
// configure a store and reach the gRPC integration with a single import.
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

// AuthFunc returns an [auth.AuthFunc] that validates the credential token
// against store. Errors from store are mapped to gRPC status codes:
//
//   - [static.ErrInvalidToken] / [static.ErrEmptyToken] → codes.Unauthenticated
//   - [static.ErrRateLimited]                          → codes.ResourceExhausted
//   - any other error                                  → codes.Internal
//
// Panics if store is nil — an unauthenticated server is not a usable default.
func AuthFunc(store TokenStore) auth.AuthFunc {
	if store == nil {
		panic("transport/grpc/.../auth/static: AuthFunc requires a non-nil TokenStore")
	}
	return func(ctx context.Context, request auth.Request) (any, error) {
		creds, ok := request.TokenCredentials()
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing token credentials")
		}
		data, err := store.Validate(ctx, creds.Token)
		if err != nil {
			return nil, toGRPCStatus(err)
		}
		return data, nil
	}
}

func toGRPCStatus(err error) error {
	switch {
	case errors.Is(err, static.ErrInvalidToken), errors.Is(err, static.ErrEmptyToken):
		return status.Error(codes.Unauthenticated, "invalid token")
	case errors.Is(err, static.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, "too many authentication attempts")
	default:
		return status.Errorf(codes.Internal, "auth: %v", err)
	}
}
