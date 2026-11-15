// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"slices"
	"time"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// validateClaims performs custom claims validation based on verifier options.
func (p *Provider) validateClaims(claims map[string]any, ops *verifierOptions, compiledCELRules []celPreCompiledValidationRule) error {
	if ops.withoutClaimsValidation {
		return nil
	}

	if err := p.checkAudienceConfigured(ops); err != nil {
		return err
	}

	// Use the pre-built ignored claims set (populated by buildIgnoredSet).
	ignored := ops.ignoredClaimsSet

	if err := validateRequiredClaims(claims, ops, ignored); err != nil {
		return err
	}

	if err := validateExpectedClaims(claims, ops, ignored); err != nil {
		return err
	}

	if err := validateAllowedClientIDs(claims, ops.allowedClientIDs); err != nil {
		return err
	}

	if err := validateRequiredScopes(claims, ops.requiredScopes); err != nil {
		return err
	}

	if err := validateAuthorizedParty(claims, ops.requireAuthorizedParty, ops.allowedAuthorizedParties); err != nil {
		return err
	}

	if err := validateTokenLifetime(claims, ops.maxTokenLifetime); err != nil {
		return err
	}

	return validateCELRules(claims, compiledCELRules)
}

// checkAudienceConfigured enforces the provider's [AudienceFailureMode] when
// validation runs without an expected audience. With an expected audience set
// its value is enforced by the jwt verifier ([Provider.jwtVerifyOptions]); the
// `aud` claim being merely present (DefaultRequiredClaims) does not bind the
// token to this service, so an unconfigured audience is treated per the mode:
// enforce rejects, warn logs and continues, disabled is silent.
func (p *Provider) checkAudienceConfigured(ops *verifierOptions) error {
	if len(ops.audience) > 0 {
		return nil
	}

	switch p.opts.audienceFailureMode {
	case AudienceFailureModeWarn:
		p.logger.Warn("token validated without an expected audience; cross-audience replay is possible " +
			"(configure WithValidationAudience or set WithAudienceFailureMode)")
		return nil
	case AudienceFailureModeDisabled:
		return nil
	default: // AudienceFailureModeEnforce
		return coreerrs.Wrap(ErrAudienceNotConfigured,
			"refusing to validate token without an expected audience (set WithValidationAudience or relax via WithAudienceFailureMode)")
	}
}

func isIgnoredClaim(ignored map[string]struct{}, claim string) bool {
	if len(ignored) == 0 {
		return false
	}
	_, exists := ignored[claim]
	return exists
}

// claimSubject is the registered JWT subject claim.
const claimSubject = "sub"

// validateRequiredClaims checks DefaultRequiredClaims plus the custom
// required claims: a custom list extends the defaults, never replaces them.
// Ignored claims skip the check, except exp, which is always required.
func validateRequiredClaims(claims map[string]any, ops *verifierOptions, ignored map[string]struct{}) error {
	for _, claim := range slices.Concat(DefaultRequiredClaims, ops.requiredClaims) {
		if claim != "exp" && isIgnoredClaim(ignored, claim) {
			continue
		}

		if claim == claimSubject && ops.allowMissingSubject {
			continue
		}

		value, exists := claims[claim]
		if !exists {
			return coreerrs.Wrapf(ErrTokenInvalid, "missing required claim '%s'", claim)
		}

		if claim == claimSubject {
			subStr, ok := value.(string)
			if !ok || subStr == "" {
				return coreerrs.Wrap(ErrTokenInvalid, "claim 'sub' cannot be empty")
			}
		}
	}

	return nil
}

func validateExpectedClaims(claims map[string]any, ops *verifierOptions, ignored map[string]struct{}) error {
	for claim, expectedValue := range ops.expectedClaims {
		if isIgnoredClaim(ignored, claim) {
			continue
		}

		actualValue, exists := claims[claim]
		if !exists {
			return coreerrs.Wrapf(ErrTokenInvalid, "missing expected claim '%s'", claim)
		}

		if !claimMatches(actualValue, expectedValue) {
			return coreerrs.Wrapf(ErrTokenInvalid, "claim '%s' has unexpected value", claim)
		}
	}

	return nil
}

func validateAllowedClientIDs(claims map[string]any, allowed []string) error {
	if len(allowed) == 0 {
		return nil
	}

	clientID, exists := claims["client_id"]
	if !exists {
		clientID, exists = claims["azp"]
	}
	if !exists {
		return coreerrs.Wrap(ErrTokenInvalid, "missing 'client_id' or 'azp' claim for client ID validation")
	}

	clientIDStr, ok := clientID.(string)
	if !ok {
		return coreerrs.Wrap(ErrTokenInvalid, "claim 'client_id' is not a string")
	}

	if !slices.Contains(allowed, clientIDStr) {
		return coreerrs.Wrap(ErrTokenInvalid, "client_id is not in the allowed list")
	}

	return nil
}

func validateRequiredScopes(claims map[string]any, requiredScopes []string) error {
	if len(requiredScopes) == 0 {
		return nil
	}

	tokenScopes, err := strictScopeClaim(claims)
	if err != nil {
		return err
	}

	for _, requiredScope := range requiredScopes {
		if slices.Contains(tokenScopes, requiredScope) {
			return nil
		}
	}

	return coreerrs.Wrap(ErrTokenInvalid, "token does not contain any of the required scopes")
}

func strictScopeClaim(claims map[string]any) ([]string, error) {
	scopeValue, exists := claims["scope"]
	if !exists {
		return nil, coreerrs.Wrap(ErrTokenInvalid, "missing 'scope' claim for scope validation")
	}

	scopes, ok := normalizeScopeValue(scopeValue)
	if !ok {
		return nil, coreerrs.Wrap(ErrTokenInvalid, "claim 'scope' has invalid type")
	}

	return scopes, nil
}

func validateAuthorizedParty(claims map[string]any, require bool, allowedParties []string) error {
	if !require {
		return nil
	}

	azp, exists := claims["azp"]
	if !exists {
		return coreerrs.Wrap(ErrTokenInvalid, "missing required 'azp' (authorized party) claim")
	}

	azpStr, ok := azp.(string)
	if !ok {
		return coreerrs.Wrap(ErrTokenInvalid, "claim 'azp' is not a string")
	}

	if len(allowedParties) > 0 && !slices.Contains(allowedParties, azpStr) {
		return coreerrs.Wrap(ErrTokenInvalid, "authorized party is not in the allowed list")
	}

	return nil
}

func validateTokenLifetime(claims map[string]any, maxLifetime time.Duration) error {
	if maxLifetime <= 0 {
		return nil
	}

	expTime, expOk := parseClaimAsInt64(claims, "exp")
	iatTime, iatOk := parseClaimAsInt64(claims, "iat")

	if !expOk || !iatOk {
		return coreerrs.Wrap(ErrTokenInvalid, "missing or invalid 'exp' or 'iat' claim for lifetime validation")
	}

	lifetime := time.Duration(expTime-iatTime) * time.Second
	if lifetime < 0 {
		return coreerrs.Wrap(ErrTokenInvalid, "invalid token lifetime (exp < iat)")
	}

	if lifetime > maxLifetime {
		return coreerrs.Wrapf(ErrTokenInvalid, "token lifetime %v exceeds maximum allowed %v", lifetime, maxLifetime)
	}

	return nil
}

func validateCELRules(claims map[string]any, compiled []celPreCompiledValidationRule) error {
	for i := range compiled {
		rule := &compiled[i]
		if rule.matcher(claims) {
			continue
		}

		// Use custom message if provided, otherwise use default format
		if rule.message != "" {
			return coreerrs.Wrapf(ErrCELValidation, "%s", rule.message)
		}
		// The rule is identified by name only: the expression is policy
		// source, and this error reaches logs and callers.
		return coreerrs.Wrapf(ErrCELValidation, "rule '%s' failed", rule.name)
	}

	return nil
}

// parseClaimAsInt64 extracts a numeric claim as int64, returning false if missing or invalid.
func parseClaimAsInt64(claims map[string]any, claimName string) (int64, bool) {
	value, ok := claims[claimName]
	if !ok {
		return 0, false
	}

	switch v := value.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	default:
		return 0, false
	}
}

// getTokenExpirationTTL returns time until token expiration, or 0 if expired/invalid.
func getTokenExpirationTTL(claims map[string]any) time.Duration {
	expTime, ok := parseClaimAsInt64(claims, "exp")
	if !ok {
		return 0
	}

	now := time.Now().Unix()
	ttl := expTime - now
	if ttl <= 0 {
		return 0
	}

	return time.Duration(ttl) * time.Second
}
