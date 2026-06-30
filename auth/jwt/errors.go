// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import "errors"

// Sentinel errors returned by [Signer] and [Verifier].
var (
	// ErrTokenInvalid is returned when a token is malformed, carries an
	// unexpected algorithm, fails signature verification, or fails claim
	// validation other than expiry.
	ErrTokenInvalid = errors.New("auth/jwt: invalid token")

	// ErrTokenExpired is returned when a token's exp claim is in the past
	// (beyond the configured clock-skew leeway).
	ErrTokenExpired = errors.New("auth/jwt: token expired")

	// ErrAlgorithmNotAllowed is returned when a token's or signing key's
	// algorithm is not in the allow-list, is not registered with golang-jwt, or
	// does not match the resolved verification key's bound algorithm.
	ErrAlgorithmNotAllowed = errors.New("auth/jwt: algorithm not allowed")

	// ErrClaimMissing is returned when a claim required via
	// [WithRequiredClaims] is absent.
	ErrClaimMissing = errors.New("auth/jwt: required claim missing")

	// ErrTokenTypeInvalid is returned when the token's typ header does not match
	// the type required via [WithExpectedTokenType] (for example a resource
	// server requiring RFC 9068 "at+jwt").
	ErrTokenTypeInvalid = errors.New("auth/jwt: token type not accepted")

	// ErrSigningKeyInvalid is returned by [Signer] when the signing material
	// cannot produce a verifiable token, such as an empty key id (a verifier
	// rejects a token whose kid header is empty).
	ErrSigningKeyInvalid = errors.New("auth/jwt: invalid signing key")

	// ErrTokenRevoked is returned when the token's jti has been revoked through
	// the [RevocationChecker] configured with [WithRevocation].
	ErrTokenRevoked = errors.New("auth/jwt: token revoked")
)
