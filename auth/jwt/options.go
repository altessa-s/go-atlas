// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"crypto/rand"
	"io"
	"slices"
	"time"
)

// Default tunables for the signer and verifier.
const (
	// DefaultIssuer is the empty iss claim; deployments override it with
	// [WithIssuer].
	DefaultIssuer = ""

	// DefaultLeeway is the clock-skew tolerance applied to exp / nbf / iat
	// verification to absorb drift between signer and verifier.
	DefaultLeeway = 30 * time.Second

	// DefaultMaxTokenLifetime is the ceiling a requested token TTL is clamped to
	// by [Signer.NewClaims]; a non-positive TTL uses this value.
	DefaultMaxTokenLifetime = 30 * 24 * time.Hour

	// DefaultExpirationRequired makes an exp claim mandatory during
	// verification, the fail-closed default.
	DefaultExpirationRequired = true

	// DefaultTokenType is the empty typ header: [Signer.Sign] then leaves the
	// golang-jwt default of "JWT" ([TypeJWT]). Set another value, such as
	// [TypeAccessToken] for RFC 9068, via [WithTokenType].
	DefaultTokenType = ""
)

// defaultRand is the production randomness source for jti generation.
var defaultRand io.Reader = rand.Reader

// options carries the signer/verifier tunables. rand and clock are injectable
// seams for deterministic tests.
type options struct {
	issuer             string        `optgen:"default=DefaultIssuer"`
	leeway             time.Duration `optgen:"default=DefaultLeeway" optval:"positive=allow_zero"`
	maxTokenLifetime   time.Duration `optgen:"default=DefaultMaxTokenLifetime"`
	expirationRequired bool          `optgen:"default=DefaultExpirationRequired"`
	allowedAlgorithms  []Algorithm   `optgen:"manual,default=DefaultAllowedAlgorithms()"`
	rand               io.Reader     `optgen:"default=defaultRand"`
	clock              Clock         `optgen:"default=defaultClock"`

	// tokenType, when non-empty, is written to the signed token's typ header by
	// [Signer.Sign] (for example [TypeAccessToken] / "at+jwt" per RFC 9068).
	// Empty leaves the golang-jwt default of "JWT". Set via [WithTokenType].
	tokenType string `optgen:"default=DefaultTokenType"`

	// expectedTokenType, when non-empty, makes [Verifier.Verify] require the
	// token's typ header to match it (case-insensitive, with or without an
	// "application/" prefix). Empty leaves typ unchecked. Set via
	// [WithExpectedTokenType].
	expectedTokenType string

	// subject, when non-empty, makes [Verifier] require the sub claim to equal
	// it. Empty leaves sub unchecked. Set via [WithSubject].
	subject string

	// issuedAt makes [Verifier] validate the iat claim, rejecting a token issued
	// in the future beyond the clock-skew leeway. Set via [WithIssuedAt].
	issuedAt bool

	// notBeforeRequired makes [Verifier] require the nbf claim to be present (it
	// is always honored when present). Set via [WithNotBeforeRequired].
	notBeforeRequired bool

	// audiences is the set of acceptable aud values; verification accepts a
	// token whose aud intersects it. Empty means the aud claim is not checked.
	// Set via [WithAudiences].
	audiences []string `opt:"-"`

	// requiredClaims names claims that must be present (beyond the temporal
	// claims golang-jwt enforces). Set via [WithRequiredClaims].
	requiredClaims []string `opt:"-"`

	// revocation, when set, rejects a verified token whose jti has been revoked,
	// after its signature and registered claims validate, with [ErrTokenRevoked].
	// A token with an empty jti is never matched. Nil leaves revocation
	// unchecked. Back it with auth/denylist or any [RevocationChecker]. Set via
	// the generated WithRevocation.
	revocation RevocationChecker `optgen:"notnil"`
}

// WithAllowedAlgorithms sets the signature algorithms the signer and verifier
// accept, replacing the default asymmetric allow-list ([DefaultAllowedAlgorithms]).
// Restricting or extending the set is the supported way to add a new algorithm:
// register a signing method with golang-jwt, then list it here. A call with no
// algorithms is ignored so the safe default is never cleared.
func WithAllowedAlgorithms(algs ...Algorithm) Option {
	return func(o *options) {
		if len(algs) == 0 {
			return
		}
		o.allowedAlgorithms = slices.Clone(algs)
	}
}

// WithAudiences sets the acceptable aud values. Verification accepts a token
// whose aud claim intersects this set; an empty set leaves aud unchecked. A
// call with no audiences is ignored.
func WithAudiences(auds ...string) Option {
	return func(o *options) {
		if len(auds) == 0 {
			return
		}
		o.audiences = slices.Clone(auds)
	}
}

// WithRequiredClaims names claims that must be present, beyond the temporal
// claims the verifier already enforces. Verification fails with [ErrClaimMissing]
// when any is absent. A call with no claims is ignored.
func WithRequiredClaims(names ...string) Option {
	return func(o *options) {
		if len(names) == 0 {
			return
		}
		o.requiredClaims = slices.Clone(names)
	}
}

// WithExpirationOptional makes the exp claim optional, reversing the
// fail-closed default ([DefaultExpirationRequired]) under which a token without
// exp is rejected. When optional, exp is still validated if present but a token
// omitting it is accepted. It is the inverse of [WithExpirationRequired]; the
// last of the two applied wins. Use it for token profiles that legitimately
// omit exp (for example a signed userinfo response).
func WithExpirationOptional() Option {
	return func(o *options) {
		o.expirationRequired = false
	}
}
