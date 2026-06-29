// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Validator defines the interface for OIDC token validation. It returns the
// verified token's structured [Claims], so a validator and the [AuthFunc] /
// [validator.DefaultValidator] built on it agree on one claim type rather than
// a raw map. Returns an error if the token is invalid, expired, or verification
// fails.
type Validator interface {
	// ValidateToken verifies the token and extracts its claims.
	ValidateToken(ctx context.Context, token string) (*Claims, error)
}

// AuthFunc creates a gRPC authentication function that validates OIDC tokens.
// This function integrates with the grpc/interceptors/auth package to provide
// OIDC-based authentication for gRPC services.
//
// The returned auth.Func performs the following steps:
//  1. Extracts the token from the request metadata (expects Bearer token format)
//  2. Validates the token using the provided Validator
//  3. Returns the extracted Claims for use in subsequent interceptors or handlers
//
// Usage example:
//
//	provider := oidctools.NewProvider(...)
//	validator := oidc.NewDefaultValidator(provider)
//	authFunc := oidc.AuthFunc(validator)
//
//	// Use with gRPC server interceptor
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(auth.UnaryServerInterceptor(authFunc)),
//	)
//
// Returns an auth.Func that can be used with gRPC server interceptors.
// Authentication failures return gRPC Unauthenticated errors.
//
// Pass [WithAudit] to record each token-validation decision through an
// [github.com/altessa-s/go-atlas/auth/audit.Recorder].
func AuthFunc(validator Validator, opts ...AuthOption) auth.AuthFunc {
	var cfg authConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	return func(ctx context.Context, request auth.Request) (any, error) {
		tokenCred, ok := request.TokenCredentials()
		if !ok {
			return nil, cfg.finalize(ctx, nil, false, "missing_token",
				status.Error(codes.Unauthenticated, "Missing token"))
		}

		claims, err := validator.ValidateToken(ctx, tokenCred.Token.Expose())
		if err != nil {
			return nil, cfg.finalize(ctx, nil, false, auditReason(err),
				status.Error(codes.Unauthenticated, "Token validation failed"))
		}
		if ferr := cfg.finalize(ctx, claims, true, "", nil); ferr != nil {
			return nil, ferr
		}
		return claims, nil
	}
}

// AuthOption configures [AuthFunc].
type AuthOption func(*authConfig)

type authConfig struct {
	recorder  *audit.Recorder
	subjectOf func(*Claims) string
}

// WithAudit records each token-validation decision through rec, with action
// "validate_token". subjectOf extracts the principal identity from the [Claims]
// returned on success; pass nil to leave the subject empty. When rec is
// configured with audit.FailureRequired and recording an otherwise-successful
// validation fails, the call is failed with codes.Internal so nothing proceeds
// unrecorded.
func WithAudit(rec *audit.Recorder, subjectOf func(*Claims) string) AuthOption {
	return func(c *authConfig) {
		c.recorder = rec
		c.subjectOf = subjectOf
	}
}

func (c authConfig) finalize(ctx context.Context, claims *Claims, allowed bool, reason string, authErr error) error {
	if c.recorder == nil {
		return authErr
	}
	subject := ""
	if allowed && c.subjectOf != nil {
		subject = c.subjectOf(claims)
	}
	recErr := c.recorder.Record(ctx, audit.Decision{
		Time:       time.Now().UTC(),
		Allowed:    allowed,
		Subject:    subject,
		Action:     "validate_token",
		Reason:     reason,
		Attributes: map[string]string{"transport": "grpc"},
	})
	if recErr != nil && authErr == nil {
		return status.Error(codes.Internal, "authentication audit failed")
	}
	return authErr
}

// auditReason maps a token-validation error to a stable audit reason token.
// Validators built on github.com/altessa-s/go-atlas/auth/oidc surface the
// provider sentinels; unrecognized errors fall back to "error".
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
