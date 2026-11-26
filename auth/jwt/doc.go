// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package jwt is a generic, low-level toolkit for signing and verifying JWTs.
//
// It is the shared core beneath the repository's specialized auth packages:
// auth/selfjwt (self-issued per-subject tokens) and auth/oidc (external
// OIDC providers) both build the same primitives — an algorithm allow-list, an
// algorithm-confusion guard, standard temporal/issuer/audience claim
// validation, and claim extraction — on top of github.com/golang-jwt/jwt/v5.
// This package factors those primitives out so a new service can sign and
// verify tokens without re-deriving the security policy, and so the
// specialized packages can converge on one canonical implementation.
//
// Verified exp, iat, and nbf values retain their signed spelling as json.Number.
// Verification compares their exact values to the configured clock and leeway.
// [Claims.Expiry] rounds upward to a nanosecond for retention deadlines.
//
// Other numbers are read without losing digits: a number that float64 holds
// exactly (every small integer, ordinary decimals) decodes as float64, any
// other, such as an identifier above 2^53, stays a json.Number with its signed
// spelling, and [VerifyInto] and [Claims.Decode] read that spelling.
//
// # Revocation
//
// [WithRevocation] consults a [RevocationChecker] with each token's jti. A
// token without a string jti is rejected ([ErrClaimMissing]) because it could
// never be revoked ([WithInsecureRevocationAllowMissingID] opts out), and a
// checker that cannot answer rejects with [ErrRevocationUnavailable], distinct
// from [ErrTokenRevoked]. Back it with a primed auth/denylist/mirror — prime it
// before serving, see factory/auth/denylist/mirror — or the in-memory
// auth/denylist.
//
// # Key resolution seam
//
// Verification does not assume where keys come from. A caller supplies a
// [KeyResolver] that maps a token's [Header] and its still-unverified [Claims]
// to a [VerificationKey]. This single seam covers both styles in the
// repository: resolving by the subject claim plus kid (selfjwt) and resolving
// by kid alone against a JWKS (oidc). JWKS fetching, caching, and rotation are
// the caller's concern; this package only consumes the resolved key.
//
// # Algorithm agnostic
//
// The package hard-codes no signature algorithm. A [SigningKey] /
// [VerificationKey] carries an [Algorithm] and a crypto key (ed25519, ECDSA,
// or RSA), and both signing and verification accept only the algorithms in the
// allow-list. The default allow-list is asymmetric only (EdDSA, ES256, RS256);
// HMAC is intentionally excluded so a symmetric secret can never be confused
// with a public key. Supporting a further algorithm is a matter of registering
// a signing method with golang-jwt and adding it via [WithSignerAllowedAlgorithms] and [WithVerifierAllowedAlgorithms] —
// no change to this package.
//
// # Usage
//
//	signer := jwt.NewSigner(jwt.WithSignerIssuer("billing"))
//	claims, _ := signer.NewClaims(tenantID, time.Hour, "files:read")
//	raw, err := signer.Sign(signingKey, claims)
//
//	verifier := jwt.NewVerifier(resolver, jwt.WithVerifierIssuer("billing"))
//	claims, err := verifier.Verify(ctx, raw)
//	subject := claims.Subject()
//
// # Security
//
// Verification is fail-closed: the algorithm is checked against the allow-list
// before the signature is verified (closing algorithm-confusion attacks), an
// exp claim is mandatory by default, and temporal claims are validated with a
// configurable clock-skew leeway. A resolved verification key may bind itself
// to one algorithm (see [VerificationKey.Algorithm]) so a key can never be
// coerced into verifying a different scheme.
package jwt
