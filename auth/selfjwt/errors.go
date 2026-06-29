// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import "errors"

// Sentinel errors returned by [Minter] and [Verifier].
var (
	// ErrTokenInvalid is returned when a token is malformed, has an unexpected
	// algorithm, fails signature verification, or fails claim validation other
	// than expiry.
	ErrTokenInvalid = errors.New("auth/selfjwt: invalid token")

	// ErrTokenExpired is returned when a token's exp claim is in the past
	// (beyond the configured clock-skew leeway).
	ErrTokenExpired = errors.New("auth/selfjwt: token expired")

	// ErrSubjectUnknown is the contract a [KeyProvider] returns when it has no
	// keys for the requested subject; [Verifier] propagates it unwrapped.
	ErrSubjectUnknown = errors.New("auth/selfjwt: subject unknown")

	// ErrKeyRotated is the contract a [KeyProvider] returns when the subject is
	// known but the requested kid is not (the signing key has been rotated
	// away); [Verifier] propagates it unwrapped.
	ErrKeyRotated = errors.New("auth/selfjwt: signing key rotated")

	// ErrAlgorithmNotAllowed is returned when a signing algorithm is not
	// registered with golang-jwt, when a token's algorithm does not match the
	// resolved verification key's algorithm, or when a resolved verification key
	// carries no algorithm to bind the token to.
	ErrAlgorithmNotAllowed = errors.New("auth/selfjwt: algorithm not allowed")

	// ErrSigningKeyInvalid is returned by [Minter] when the [KeyProvider]
	// returns signing material that cannot produce a verifiable token, such as
	// an empty key id (the verifier rejects a token whose kid header is empty).
	ErrSigningKeyInvalid = errors.New("auth/selfjwt: invalid signing key")

	// ErrSubjectRequired is returned by [Minter] when a [MintRequest] carries an
	// empty subject. A subject is mandatory: an empty sub yields a token the
	// verifier always rejects (resolveKey requires a non-empty subject).
	ErrSubjectRequired = errors.New("auth/selfjwt: subject required")
)
