// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=verifierOptions --option-type=ValidationOption --output=verifier_options_gen.go --option-prefix=Validation

import (
	"slices"
	"time"
)

// DefaultValidMethods is the set of JWT signing algorithms allowed by default.
// Only asymmetric algorithms are included to prevent algorithm confusion attacks
// where an attacker forges a token using HMAC with a public key.
//
// Excluded algorithms:
//   - "none" — no signature (trivially forgeable)
//   - HS256, HS384, HS512 — HMAC symmetric (vulnerable to public-key confusion)
var DefaultValidMethods = []string{
	"RS256", "RS384", "RS512", // RSA PKCS1 v1.5
	"ES256", "ES384", "ES512", // ECDSA
	"PS256", "PS384", "PS512", // RSA-PSS
	"EdDSA", // Edwards-curve DSA
}

// WithValidationValidMethods restricts JWT signing algorithms to the given list.
// If not called, or called with an empty list, DefaultValidMethods is used.
func WithValidationValidMethods(methods ...string) ValidationOption {
	return func(o *verifierOptions) {
		if len(methods) == 0 {
			return
		}
		o.validMethods = slices.Clone(methods)
	}
}

// verifierOptions holds token validation configuration.
// This struct is not exported and is modified through ValidationOption functions.
type verifierOptions struct {
	withoutClaimsValidation  bool `opt:"ClaimsValidation" optval:"invert"`
	leeway                   time.Duration
	issuedAt                 bool
	issuer                   string
	subject                  string
	audience                 []string
	requiredClaims           []string
	expectedClaims           map[string]string
	ignoredClaims            []string
	allowedClientIDs         []string
	requiredScopes           []string
	requireAuthorizedParty   bool
	allowedAuthorizedParties []string
	allowMissingSubject      bool
	maxTokenLifetime         time.Duration
	celRules                 []CELValidationRule
	validMethods             []string            `opt:"-"` // Allowed JWT signing algorithms. Empty = DefaultValidMethods.
	ignoredClaimsSet         map[string]struct{} `opt:"-"` // Pre-built from ignoredClaims. Call buildIgnoredSet() after finalization.
}

// buildIgnoredSet pre-computes ignoredClaimsSet from ignoredClaims.
func (o *verifierOptions) buildIgnoredSet() {
	if len(o.ignoredClaims) == 0 {
		o.ignoredClaimsSet = nil
		return
	}
	o.ignoredClaimsSet = make(map[string]struct{}, len(o.ignoredClaims))
	for _, claim := range o.ignoredClaims {
		o.ignoredClaimsSet[claim] = struct{}{}
	}
}
