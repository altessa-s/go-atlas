// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"slices"

	authjwt "github.com/altessa-s/go-atlas/auth/jwt"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

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
func (p *Provider) newValidationPolicy(ops *verifierOptions, celRules []celPreCompiledValidationRule) *validationPolicy {
	policy := &validationPolicy{
		ops:      ops,
		verifier: authjwt.NewVerifier(p.keyResolver, p.jwtVerifyOptions(ops)...),
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
func (p *Provider) compileDefaultPolicies() {
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
func (vp *validationPolicy) signatureVerifier(p *Provider) *authjwt.Verifier {
	if len(vp.ops.validMethods) == 0 && !slices.Equal(vp.defaultMethods, DefaultValidMethods) {
		return authjwt.NewVerifier(p.keyResolver, p.jwtVerifyOptions(vp.ops)...)
	}
	return vp.verifier
}

// applyPolicy validates signature-verified claims against policy: the
// registered claims (exp/nbf/iat with leeway, iss, sub, aud) and the OIDC
// checks of [Provider.validateClaims]. The algorithm allow-list belongs to the
// signature step and is not re-applied here.
func (p *Provider) applyPolicy(claims map[string]any, policy *validationPolicy) error {
	if policy.ops.withoutClaimsValidation {
		return nil
	}
	if err := policy.verifier.ValidateClaims(authjwt.Claims(claims)); err != nil {
		return coreerrs.Wrapf(ErrTokenInvalid, "%s", err)
	}
	return p.validateClaims(claims, policy.ops, policy.celRules)
}
