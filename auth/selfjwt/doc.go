// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package selfjwt mints and verifies self-issued JWTs.
//
// Unlike auth/oidc, which validates tokens issued by an external identity
// provider against a remote JWKS, selfjwt is for services that are both the
// issuer and the verifier of their own per-subject tokens: a service signs a
// token for a tenant (or any subject) with that subject's private key, embeds
// the key id in the token header, and later verifies it by resolving the
// subject's public key for that kid. Rotating a subject's signing key (issuing
// a new kid) invalidates every token still carrying the old kid.
//
// # Algorithm agnostic
//
// The package does not hard-code a signature algorithm. A [KeyProvider]
// returns a [SigningKey] / [VerificationKey] carrying an [Algorithm] and a
// crypto key (ed25519, ECDSA, or RSA), and the verifier accepts only the
// algorithms in its allow-list. The default allow-list is asymmetric only
// (EdDSA, ES256, RS256); HMAC is intentionally excluded so a symmetric secret
// can never be confused with a public key. Support for further algorithms is a
// matter of registering a signing method with golang-jwt and adding it to the
// allow-list via [WithAllowedAlgorithms] — no change to this package.
//
// # Usage
//
//	minter := selfjwt.NewMinter(provider, selfjwt.WithIssuer("billing"))
//	res, err := minter.Mint(ctx, selfjwt.MintRequest{Subject: tenantID, Scopes: []string{"files:read"}, TTL: time.Hour})
//
//	verifier := selfjwt.NewVerifier(provider, selfjwt.WithIssuer("billing"))
//	tok, err := verifier.Verify(ctx, res.Token)
//
// # Security
//
// Verification is fail-closed: the algorithm is checked against the allow-list
// before the signature is verified (closing algorithm-confusion attacks), an
// exp claim is mandatory, and temporal claims are validated with a configurable
// clock-skew leeway. Resolved verification keys are cached per (subject, kid)
// with a TTL via [WithCacheTTL].
package selfjwt
