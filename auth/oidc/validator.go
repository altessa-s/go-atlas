// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/internal/diagnostics"
)

// TokenVerifier verifies a raw token and returns its verified claim set.
// [*Provider] satisfies it; the narrow interface lets a [Validator] run over
// a test double or any other verifier.
type TokenVerifier interface {
	ValidateToken(ctx context.Context, token string) (map[string]any, error)
}

// ClaimsValidator verifies a token and returns its typed [Claims]. It is the
// port the HTTP and gRPC OIDC adapters consume; [*Validator] satisfies it.
type ClaimsValidator interface {
	ValidateToken(ctx context.Context, token string) (*Claims, error)
}

var (
	_ TokenVerifier   = (*Provider)(nil)
	_ ClaimsValidator = (*Validator)(nil)
)

// Validator verifies tokens through a [TokenVerifier] and projects the
// verified claim set onto [Claims] using the standard OpenID Connect Core 1.0
// claim names (sub, preferred_username, email, iss, aud, scope, exp, iat, nbf,
// family_name, name, given_name). The raw claim map is kept in
// Claims.RawClaims. A Validator is immutable and safe for concurrent use.
//
// Verification errors are returned wrapped, never mapped to a transport
// status: [errors.Is] still matches the [Provider] sentinels such as
// [ErrTokenInvalid] and [ErrTokenRevoked], and the transport adapters decide
// how a failure reaches the caller. A failure is logged at Debug when the token
// was rejected ([ErrTokenInvalid], [ErrCELValidation], [ErrTokenRevoked]) and
// at Error otherwise, which points at the verifier's infrastructure.
type Validator struct {
	verifier TokenVerifier
	logger   *slog.Logger
}

// NewValidator returns a [Validator] over verifier, typically a [*Provider].
func NewValidator(verifier TokenVerifier, opts ...ValidatorOption) *Validator {
	o := newValidatorOptions(opts...)
	return &Validator{verifier: verifier, logger: o.logger}
}

// ValidateToken verifies token and extracts its [Claims]. Scopes keep their
// token order and accept a space-separated string or an array; claim parsing is
// delegated to the shared github.com/altessa-s/go-atlas/auth/jwt accessors.
func (v *Validator) ValidateToken(ctx context.Context, token string) (*Claims, error) {
	raw, err := v.verifier.ValidateToken(ctx, token)
	if err != nil {
		logValidationFailure(ctx, v.logger, "failed to verify token", err)
		return nil, fmt.Errorf("oidc: validate token: %w", err)
	}
	return claimsFromRaw(raw), nil
}

// isTokenRejection reports whether err is the ordinary outcome of validating an
// untrusted token — malformed, expired, badly signed, failing a claim or CEL
// rule, or revoked — as opposed to a failure of the verifier's own
// infrastructure (a stale JWKS, an unreachable introspection endpoint or
// revocation storage).
func isTokenRejection(err error) bool {
	return errors.Is(err, ErrTokenInvalid) || errors.Is(err, ErrCELValidation) || errors.Is(err, ErrTokenRevoked)
}

// logValidationFailure logs a failed token validation at the level its cause
// deserves: Debug for a rejected token, which any caller can provoke at will and
// must not be able to flood the error log with, and Error for everything else,
// which points at the verifier's infrastructure.
func logValidationFailure(ctx context.Context, logger *slog.Logger, msg string, err error) {
	level := slog.LevelError
	if isTokenRejection(err) {
		level = slog.LevelDebug
	}
	logger.Log(ctx, level, msg, slog.String("error", validationClass(err)))
}

// validationClass returns only fixed local diagnostics, even for an injected verifier.
func validationClass(err error) string {
	for _, kind := range []error{ErrTokenInvalid, ErrCELValidation, ErrTokenRevoked, ErrJWKSStale, ErrRevocationStale, ErrRevocationCheck, ErrIntrospection} {
		if errors.Is(err, kind) {
			return kind.Error()
		}
	}
	return diagnostics.ErrorClass(err)
}
