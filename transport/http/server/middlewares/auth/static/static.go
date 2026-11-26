// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static

import (
	"context"
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/auth/static"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
	"github.com/altessa-s/go-atlas/transport/internal/authaudit"
)

// AuthFunc returns an [auth.AuthFunc] that validates the token against store.
// Errors from store are wrapped so callers can match both the transport-level
// [auth.ErrUnauthorized] / [auth.ErrInvalidToken] sentinels and the underlying
// [static.ErrTokenInvalid] / [static.ErrTokenEmpty] / [static.ErrRateLimited] /
// [static.ErrRateLimitKeyEmpty]
// causes via [errors.Is]. The original error chain is preserved for logs and
// custom [auth.ErrorHandler] implementations.
//
// Pass [WithAudit] to record each authentication decision through an
// [auth.DecisionRecorder].
//
// Panics if store is nil — an unauthenticated middleware is not a usable
// default.
func AuthFunc(store static.TokenStore, opts ...AuthOption) auth.AuthFunc {
	if store == nil {
		panic("transport/http/.../auth/static: AuthFunc requires a non-nil TokenStore")
	}
	var cfg authConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return auth.AuthenticateFunc(func(ctx context.Context, token string) (any, error) {
		data, err := store.Validate(ctx, token)
		if err != nil {
			_ = cfg.record(ctx, nil, false, auditReason(err))
			return nil, translateError(err)
		}
		if recErr := cfg.record(ctx, data, true, ""); recErr != nil {
			return nil, recErr
		}
		return data, nil
	})
}

// AuthOption configures [AuthFunc].
type AuthOption func(*authConfig)

type authConfig struct {
	recorder  auth.DecisionRecorder
	subjectOf func(any) string
}

// WithAudit records each authentication decision through rec, with action
// "authenticate". subjectOf extracts the principal identity from the value the
// store returned on success; pass nil to leave the subject empty. When rec is
// configured with decision.FailureRequired and recording an otherwise-successful
// authentication fails, that error is returned so the request fails closed
// (nothing proceeds unrecorded).
func WithAudit(rec auth.DecisionRecorder, subjectOf func(any) string) AuthOption {
	return func(c *authConfig) {
		c.recorder = rec
		c.subjectOf = subjectOf
	}
}

func (c authConfig) record(ctx context.Context, data any, allowed bool, reason string) error {
	return authaudit.RecordAuthentication(ctx, c.recorder, "http", "authenticate", data, c.subjectOf, allowed, reason)
}

// auditReason maps a store error to a stable audit reason token.
func auditReason(err error) string {
	switch {
	case errors.Is(err, static.ErrTokenInvalid):
		return "invalid_token"
	case errors.Is(err, static.ErrTokenEmpty):
		return "empty_token"
	case errors.Is(err, static.ErrRateLimited):
		return "rate_limited"
	case errors.Is(err, static.ErrRateLimitKeyEmpty):
		return "empty_rate_limit_key"
	default:
		return "error"
	}
}

func translateError(err error) error {
	switch {
	case errors.Is(err, static.ErrTokenInvalid), errors.Is(err, static.ErrTokenEmpty), errors.Is(err, static.ErrRateLimitKeyEmpty):
		return fmt.Errorf("%w: %w", auth.ErrUnauthorized, err)
	case errors.Is(err, static.ErrRateLimited):
		return fmt.Errorf("%w: %w", auth.ErrUnauthorized, err)
	default:
		return fmt.Errorf("auth: %w", err)
	}
}
