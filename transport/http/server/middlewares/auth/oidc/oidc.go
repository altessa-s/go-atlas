// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth"
	"github.com/altessa-s/go-atlas/transport/internal/authaudit"
)

// AuthFunc returns an [auth.AuthFunc] that validates the bearer token with
// validator (typically oidc.NewValidator over an oidc.Provider) and, on
// success, returns the verified *oidc.Claims as the request's authenticated
// data. Every validation failure is wrapped with [auth.ErrUnauthorized], so
// the auth middleware answers 401; the original cause (for example
// oidc.ErrTokenRevoked) stays reachable through [errors.Is] for logs and
// custom [auth.ErrorHandler] implementations.
//
// Pass [WithAudit] to record each token-validation decision through an
// [auth.DecisionRecorder].
//
// Panics if validator is nil — an unauthenticated middleware is not a usable
// default.
func AuthFunc(validator oidc.ClaimsValidator, opts ...AuthOption) auth.AuthFunc {
	if validator == nil {
		panic("transport/http/.../auth/oidc: AuthFunc requires a non-nil ClaimsValidator")
	}
	var cfg authConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return auth.AuthenticateFunc(func(ctx context.Context, token string) (any, error) {
		claims, err := validator.ValidateToken(ctx, token)
		if err != nil {
			_ = cfg.record(ctx, nil, false, auditReason(err))
			return nil, fmt.Errorf("%w: %w", auth.ErrUnauthorized, err)
		}
		if recErr := cfg.record(ctx, claims, true, ""); recErr != nil {
			return nil, recErr
		}
		return claims, nil
	})
}

// AuthOption configures [AuthFunc].
type AuthOption func(*authConfig)

type authConfig struct {
	recorder  auth.DecisionRecorder
	subjectOf func(*oidc.Claims) string
}

// WithAudit records each token-validation decision through rec, with action
// "validate_token". subjectOf extracts the principal identity from the
// *oidc.Claims returned on success; pass nil to leave the subject empty. When
// rec is configured with decision.FailureRequired and recording an
// otherwise-successful validation fails, that error is returned so the request
// fails closed (nothing proceeds unrecorded).
func WithAudit(rec auth.DecisionRecorder, subjectOf func(*oidc.Claims) string) AuthOption {
	return func(c *authConfig) {
		c.recorder = rec
		c.subjectOf = subjectOf
	}
}

func (c authConfig) record(ctx context.Context, claims *oidc.Claims, allowed bool, reason string) error {
	return authaudit.RecordAuthentication(ctx, c.recorder, "http", "validate_token", claims, c.subjectOf, allowed, reason)
}

// auditReason maps a token-validation error to a stable audit reason token.
func auditReason(err error) string {
	switch {
	case errors.Is(err, oidc.ErrTokenRevoked):
		return "revoked"
	case errors.Is(err, oidc.ErrTokenInvalid):
		return "invalid_token"
	default:
		return "error"
	}
}
