// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altessa-s/go-atlas/auth/oidc/rules"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
)

type policyEvaluator struct {
	info                 *discoveryInfo
	keys                 *jwksKeySource
	logger               *slog.Logger
	metrics              *oidcMetrics
	tokenCache           Cacher
	tokensCacheKeyPrefix string
	audienceFailureMode  AudienceFailureMode
	verifierOptions      *verifierOptions
	celCompiledRules     []celPreCompiledValidationRule
	defaultPolicy        *validationPolicy
	builtinPolicy        *validationPolicy
	presets              map[string]*ValidationPreset
	presetRules          []PresetRule
}

// validationPolicy is a compiled validation option set: the verifier options,
// the JWT verifier built from them and the compiled CEL rules. The provider
// builds one for its default options and one per preset at construction, and
// one per call for per-call options.
type validationPolicy struct {
	ops      *verifierOptions
	verifier *authjwt.Verifier
	celRules []celPreCompiledValidationRule
	// defaultMethods is the DefaultValidMethods content verifier was built
	// with when ops sets no validMethods (nil otherwise). The exported list is
	// mutable, so signatureVerifier reuses verifier only while it is unchanged.
	defaultMethods []string
}

// newValidationPolicy builds the policy for ops. ops must be final: its
// ignored-claims set built and its CEL rules compiled into celRules.
func (p *policyEvaluator) newValidationPolicy(ops *verifierOptions, celRules []celPreCompiledValidationRule) *validationPolicy {
	policy := &validationPolicy{
		ops:      ops,
		verifier: authjwt.NewVerifier(p.keys.keyResolver, p.jwtVerifyOptions(ops)...),
		celRules: celRules,
	}
	if len(ops.validMethods) == 0 {
		policy.defaultMethods = slices.Clone(DefaultValidMethods)
	}
	return policy
}

// compileDefaultPolicies builds defaultPolicy and builtinPolicy. It runs in
// NewProvider after discovery and JWKS initialization (the verifiers bind the
// discovery issuer and the key resolver) and after validateCELRules compiled
// the default CEL rules.
func (p *policyEvaluator) compileDefaultPolicies() {
	p.builtinPolicy = p.newValidationPolicy(defaultVerifierOptions(), nil)
	p.defaultPolicy = p.builtinPolicy
	if p.verifierOptions != nil {
		p.defaultPolicy = p.newValidationPolicy(p.verifierOptions, p.celCompiledRules)
	}
}

// signatureVerifier returns the verifier for the signature step. It is the
// prebuilt one unless the policy relies on DefaultValidMethods and that list
// changed since the policy was built; then a verifier is built from the
// current options, so the algorithm allow-list always reflects the current
// defaults.
func (vp *validationPolicy) signatureVerifier(p *policyEvaluator) *authjwt.Verifier {
	if len(vp.ops.validMethods) == 0 && !slices.Equal(vp.defaultMethods, DefaultValidMethods) {
		return authjwt.NewVerifier(p.keys.keyResolver, p.jwtVerifyOptions(vp.ops)...)
	}
	return vp.verifier
}

// applyPolicy validates signature-verified claims against policy: the
// registered claims (exp/nbf/iat with leeway, iss, sub, aud) and the OIDC
// checks of [policyEvaluator.validateClaims]. The algorithm allow-list belongs to the
// signature step and is not re-applied here.
func (p *policyEvaluator) applyPolicy(claims map[string]any, policy *validationPolicy) error {
	if policy.ops.withoutClaimsValidation {
		return nil
	}
	if err := policy.verifier.ValidateClaims(authjwt.Claims(claims)); err != nil {
		return coreerrs.Wrapf(ErrTokenInvalid, "%s", err)
	}
	return p.validateClaims(claims, policy.ops, policy.celRules)
}

// prepareDefaultVerifierOptions checks the default validation options and
// pre-builds their ignored-claims lookup set.
func (p *policyEvaluator) prepareDefaultVerifierOptions() error {
	if p.verifierOptions == nil {
		return nil
	}
	if err := rules.CheckLeeway(p.verifierOptions.leeway); err != nil {
		return coreerrs.Wrap(err, "default validation options")
	}
	if err := rules.CheckTokenLifetime(p.verifierOptions.minTokenLifetime, p.verifierOptions.maxTokenLifetime); err != nil {
		return coreerrs.Wrap(err, "default validation options")
	}
	p.verifierOptions.buildIgnoredSet()
	return nil
}

// jwtVerifyOptions converts verifier options to authjwt verifier options. It
// maps only the registered-claim knobs authjwt enforces (algorithms, leeway,
// exp/nbf/iat, issuer, subject, audience); the OIDC-specific checks
// (required/expected/ignored claims, scopes, authorized party, CEL rules) stay
// in [policyEvaluator.validateClaims]. Claims validation is toggled at the call site
// via VerifySignature vs VerifyWithHeader, so withoutClaimsValidation is not
// translated here.
func (p *policyEvaluator) jwtVerifyOptions(ops *verifierOptions) []authjwt.VerifierOption {
	if ops == nil {
		return nil
	}

	// Restrict allowed signing algorithms to prevent algorithm confusion attacks.
	// Use configured methods if set, otherwise fall back to DefaultValidMethods.
	validMethods := ops.validMethods
	if len(validMethods) == 0 {
		validMethods = DefaultValidMethods
	}
	verifyOpts := []authjwt.VerifierOption{authjwt.WithVerifierAllowedAlgorithms(toAuthAlgorithms(validMethods)...)}

	// Clock-skew tolerance. Passed unconditionally so ops.leeway == 0 yields
	// strict, no-skew temporal validation (authjwt's WithLeeway accepts zero)
	// instead of falling back to authjwt's 30s default.
	verifyOpts = append(verifyOpts, authjwt.WithLeeway(ops.leeway))

	// The exp claim is always required: no validation option can accept a
	// token that never expires.
	verifyOpts = append(verifyOpts, authjwt.WithExpirationRequired())

	// Same contract for the nbf claim — `verify_not_before: true` in the
	// service config translates to "nbf must be present and respected".
	verifyOpts = coreslices.AppendIf(verifyOpts, ops.notBeforeRequired, authjwt.WithNotBeforeRequired())

	// Verify issued-at claim if requested
	verifyOpts = coreslices.AppendIf(verifyOpts, ops.issuedAt, authjwt.WithIssuedAt())

	// Always verify issuer (use discovery issuer as fallback)
	iss := ops.issuer
	if iss == "" && p.info != nil {
		iss = p.info.Issuer
	}
	verifyOpts = coreslices.AppendIf(verifyOpts, iss != "", authjwt.WithVerifierIssuer(iss))

	// Verify subject if specified
	verifyOpts = coreslices.AppendIf(verifyOpts, ops.subject != "", authjwt.WithSubject(ops.subject))

	// Verify audience if specified
	verifyOpts = coreslices.AppendIf(verifyOpts, len(ops.audience) > 0, authjwt.WithAudiences(ops.audience...))

	return verifyOpts
}

// parseAndValidateToken verifies token's signature (through the token cache)
// and validates its claims against policy. Returns the verified claims and
// token header.
func (p *policyEvaluator) parseAndValidateToken(ctx context.Context, token string, policy *validationPolicy) (jwt.MapClaims, map[string]any, error) {
	claims, header, err := p.verifyTokenSignature(ctx, token, policy)
	if err != nil {
		return nil, nil, coreerrs.Wrapf(ErrTokenInvalid, "%s", err)
	}
	if err := p.applyPolicy(claims, policy); err != nil {
		return nil, nil, err
	}
	return claims, header, nil
}

func cloneVerifierOptions(base *verifierOptions) *verifierOptions {
	if base == nil {
		return defaultVerifierOptions()
	}

	cloned := *base

	cloned.audience = slices.Clone(base.audience)
	cloned.requiredClaims = slices.Clone(base.requiredClaims)
	cloned.ignoredClaims = slices.Clone(base.ignoredClaims)
	cloned.allowedClientIDs = slices.Clone(base.allowedClientIDs)
	cloned.requiredScopes = slices.Clone(base.requiredScopes)
	cloned.allowedAuthorizedParties = slices.Clone(base.allowedAuthorizedParties)
	cloned.celRules = slices.Clone(base.celRules)
	cloned.validMethods = slices.Clone(base.validMethods)
	cloned.expectedClaims = nil

	if base.expectedClaims != nil {
		cloned.expectedClaims = maps.Clone(base.expectedClaims)
	}

	// Re-build the cached set since ignoredClaims may have been modified.
	cloned.buildIgnoredSet()

	return &cloned
}

func applyValidationOptions(ops *verifierOptions, options ...ValidationOption) {
	for _, opt := range options {
		opt(ops)
	}
}

// headerToMap converts a verified authjwt.Header into the map[string]any header
// shape the revocation and caching paths consume.
func headerToMap(hdr authjwt.Header) map[string]any {
	return map[string]any{"alg": hdr.Alg, "kid": hdr.Kid, "typ": hdr.Typ}
}

// toAuthAlgorithms converts a list of JWT method names into the [authjwt.Algorithm]
// allow-list the verifier expects.
func toAuthAlgorithms(methods []string) []authjwt.Algorithm {
	algs := make([]authjwt.Algorithm, len(methods))
	for i, m := range methods {
		algs[i] = authjwt.Algorithm(m)
	}
	return algs
}

// algorithmAllowed reports whether alg is permitted by ops. An empty validMethods
// set falls back to DefaultValidMethods, mirroring [policyEvaluator.jwtVerifyOptions].
func algorithmAllowed(ops *verifierOptions, alg string) bool {
	methods := ops.validMethods
	if len(methods) == 0 {
		methods = DefaultValidMethods
	}
	return slices.Contains(methods, alg)
}

// checkValidatable rejects an empty token and, per [WithJWKSMaxStaleness], a
// stale key set. It runs before any (cached) signature verification so cached
// verifications do not bypass the staleness budget.
func (p *policyEvaluator) checkValidatable(ctx context.Context, token string) error {
	if token == "" {
		return coreerrs.Wrap(ErrTokenInvalid, "token is empty")
	}
	// See [jwksKeySource.checkJWKSStaleness] for the per-mode behavior.
	return p.keys.checkJWKSStaleness(ctx)
}

// evaluate applies the caller options or selects a preset from verified claims.
func (p *policyEvaluator) evaluate(ctx context.Context, token string, opt ...ValidationOption) (jwt.MapClaims, map[string]any, error) {
	if err := p.checkValidatable(ctx, token); err != nil {
		return nil, nil, err
	}

	// Fast path: no presets, no default options, and no overrides
	if len(opt) == 0 && p.verifierOptions == nil && len(p.presetRules) == 0 {
		return p.parseAndValidateToken(ctx, token, p.defaultPolicy)
	}

	// No per-call options and preset selection rules configured: select the
	// preset from the signature-verified claims and validate those claims
	// with it, or with the default options when no registered preset applies.
	// The signature is verified once either way.
	if len(opt) == 0 && len(p.presetRules) > 0 {
		claims, header, preset, err := p.verifyAndSelectPreset(ctx, token)
		if err != nil {
			return nil, nil, err
		}
		policy, scope := p.defaultPolicy, ""
		if preset != nil {
			policy, scope = preset.policy, fmt.Sprintf(" by preset '%s'", preset.name)
		}
		if err := p.validateVerified(ctx, claims, header, policy, scope); err != nil {
			return nil, nil, err
		}
		return claims, header, nil
	}

	// Without per-call overrides reuse the pre-compiled default policy;
	// otherwise compile the per-call options.
	policy := p.defaultPolicy
	if len(opt) > 0 {
		ops := cloneVerifierOptions(p.verifierOptions)
		applyValidationOptions(ops, opt...)
		if err := rules.CheckTokenLifetime(ops.minTokenLifetime, ops.maxTokenLifetime); err != nil {
			return nil, nil, err
		}
		ops.buildIgnoredSet()
		compiledCELRules, err := compileVerifierCELRules(ops)
		if err != nil {
			return nil, nil, err
		}
		policy = p.newValidationPolicy(ops, compiledCELRules)
	}

	claims, header, err := p.parseAndValidateToken(ctx, token, policy)
	if err != nil {
		logValidationFailure(ctx, p.logger, "failed to validate token", err)
		return nil, nil, err
	}

	return claims, header, nil
}

// verifyAndSelectPreset verifies token's signature against the built-in
// algorithm set — the preset is only known after inspecting the claims — and
// selects the preset for the verified claims. preset is nil when no rule
// matches or the matched preset is not registered.
//
// The selection rules see a deep copy of the claims, so a matcher that
// modifies its argument cannot change the claims that are validated,
// checked for revocation and returned.
func (p *policyEvaluator) verifyAndSelectPreset(ctx context.Context, token string) (jwt.MapClaims, map[string]any, *ValidationPreset, error) {
	claims, header, err := p.verifyTokenSignature(ctx, token, p.builtinPolicy)
	if err != nil {
		err = coreerrs.Wrapf(ErrTokenInvalid, "signature verification failed: %v", err)
		logValidationFailure(ctx, p.logger, "signature verification failed", err)
		return nil, nil, nil, err
	}

	presetName := p.selectPresetForClaims(cloneClaims(claims))
	if presetName == "" {
		return claims, header, nil, nil
	}
	preset := p.getPreset(presetName)
	if preset == nil {
		p.logger.WarnContext(ctx, "preset rule matched but preset not found, using default validation",
			"preset_name", presetName)
	}
	return claims, header, preset, nil
}

// validateVerified validates signature-verified claims against policy: the
// policy's signing-algorithm allow-list on the verified header (the signature
// was verified against the built-in set), then the claim policy.
// scope qualifies the algorithm error (e.g. " by preset 'x'").
func (p *policyEvaluator) validateVerified(
	ctx context.Context,
	claims jwt.MapClaims,
	header map[string]any,
	policy *validationPolicy,
	scope string,
) error {
	alg, _ := header["alg"].(string)
	if !algorithmAllowed(policy.ops, alg) {
		return coreerrs.Wrapf(ErrTokenInvalid, "signing algorithm %q not allowed%s", alg, scope)
	}
	if err := p.applyPolicy(claims, policy); err != nil {
		logValidationFailure(ctx, p.logger, "failed to validate token", err)
		return err
	}
	return nil
}

// cloneClaims deep-copies a claim set: nested objects and arrays are copied,
// scalar values (immutable in decoded JSON) are shared.
func cloneClaims(claims map[string]any) map[string]any {
	out := make(map[string]any, len(claims))
	for k, v := range claims {
		out[k] = cloneClaimValue(v)
	}
	return out
}

func cloneClaimValue(v any) any {
	switch tv := v.(type) {
	case map[string]any:
		return cloneClaims(tv)
	case []any:
		out := make([]any, len(tv))
		for i, e := range tv {
			out[i] = cloneClaimValue(e)
		}
		return out
	default:
		return v
	}
}
