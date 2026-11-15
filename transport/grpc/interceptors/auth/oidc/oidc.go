// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AuthFunc creates a gRPC authentication function that validates OIDC tokens.
// This function integrates with the grpc/interceptors/auth package to provide
// OIDC-based authentication for gRPC services.
//
// The returned auth.Func performs the following steps:
//  1. Extracts the token from the request metadata (expects Bearer token format)
//  2. Validates the token using the provided [oidc.ClaimsValidator]
//  3. Returns the extracted *oidc.Claims for use in subsequent interceptors or handlers
//
// Usage example:
//
//	// provider is a *github.com/altessa-s/go-atlas/auth/oidc.Provider.
//	authFunc := grpcoidc.AuthFunc(oidc.NewValidator(provider))
//
//	// Use with gRPC server interceptor
//	server := grpc.NewServer(
//	    grpc.UnaryInterceptor(auth.ServerUnaryInterceptor(auth.WithAuthFn(authFunc))),
//	)
//
// Returns an auth.Func that can be used with gRPC server interceptors.
// Every validation failure — the validator returns plain errors — is mapped to
// a codes.Unauthenticated status.
//
// Pass [WithAudit] to record each token-validation decision through an
// [auth.DecisionRecorder].
func AuthFunc(validator oidc.ClaimsValidator, opts ...AuthOption) auth.AuthFunc {
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
	recorder  auth.DecisionRecorder
	subjectOf func(*oidc.Claims) string
}

// WithAudit records each token-validation decision through rec, with action
// "validate_token". subjectOf extracts the principal identity from the [oidc.Claims]
// returned on success; pass nil to leave the subject empty. When rec is
// configured with decision.FailureRequired and recording an otherwise-successful
// validation fails, the call is failed with codes.Internal so nothing proceeds
// unrecorded.
func WithAudit(rec auth.DecisionRecorder, subjectOf func(*oidc.Claims) string) AuthOption {
	return func(c *authConfig) {
		c.recorder = rec
		c.subjectOf = subjectOf
	}
}

func (c authConfig) finalize(ctx context.Context, claims *oidc.Claims, allowed bool, reason string, authErr error) error {
	if c.recorder == nil {
		return authErr
	}
	subject := ""
	if allowed && c.subjectOf != nil {
		subject = c.subjectOf(claims)
	}
	recErr := c.recorder.Record(ctx, decision.Decision{
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
