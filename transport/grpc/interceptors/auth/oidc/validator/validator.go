// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validator

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Provider is the minimal interface required from an OIDC provider implementation.
type Provider interface {
	ValidateToken(context.Context, string) (map[string]any, error)
}

// DefaultValidator satisfies [oidc.Validator]: it returns the verified token's
// structured [oidc.Claims], so it drops straight into [oidc.AuthFunc].
var _ oidc.Validator = (*DefaultValidator)(nil)

// DefaultValidator validates tokens using an OIDC Provider and extracts
// standard OIDC claims from the token. It uses fixed claim keys that follow
// the OpenID Connect Core 1.0 specification.
type DefaultValidator struct {
	provider Provider
	logger   *slog.Logger
}

// NewDefaultValidator creates a new DefaultValidator with the given provider and options.
// The validator extracts standard OIDC claims using the following keys:
//
// Standard claim keys used:
//   - Subject: "sub"
//   - Preferred Username: "preferred_username"
//   - Email: "email"
//   - Issuer: "iss"
//   - Audience: "aud"
//   - Scopes: "scope"
//   - Expires At: "exp"
//   - Issued At: "iat"
//   - Not Before: "nbf"
//   - Family Name: "family_name"
//   - Name: "name"
//   - Given Name: "given_name"
//
// Available options:
//   - WithLogger: Set a custom logger (default: no-op logger)
func NewDefaultValidator(p Provider, opts ...Option) *DefaultValidator {
	o := newOptions(opts...)
	return &DefaultValidator{
		provider: p,
		logger:   o.logger,
	}
}

// ValidateToken verifies the OIDC token using the configured provider and extracts
// structured claims from it. The method performs the following steps:
//
//  1. Verifies the token signature and validity via the OIDC provider
//  2. Extracts standard OIDC claims (sub, iss, aud, exp, iat, nbf)
//  3. Extracts user information claims (preferred_username, name, email, etc.)
//  4. Extracts OAuth 2.0 scopes from the "scope" claim
//
// The scopes claim is extracted from the "scope" key and can be provided as:
//   - A space-separated string (e.g., "openid profile email")
//   - An array of strings (e.g., ["openid", "profile", "email"])
//   - An array of mixed types (non-string values are filtered out)
//
// Scopes are automatically sorted alphabetically for consistent ordering.
//
// Returns a Claims struct containing all extracted information, or an error
// if token verification fails. Errors are logged using the configured logger.
func (v *DefaultValidator) ValidateToken(ctx context.Context, token string) (*oidc.Claims, error) {
	rawClaims, err := v.provider.ValidateToken(ctx, token)
	if err != nil {
		v.logger.ErrorContext(ctx, "failed to verify token", slog.Any("error", err))
		return nil, status.Error(codes.Unauthenticated, "Token validation failed")
	}

	claims := extractClaims(rawClaims)

	return claims, nil
}
