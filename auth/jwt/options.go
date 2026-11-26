// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

//go:generate go tool optgen generate --type=signerOptions --option-type=SignerOption --output=signer_options_gen.go
//go:generate go tool optgen generate --type=verifierOptions --option-type=VerifierOption --output=verifier_options_gen.go

import (
	"crypto/rand"
	"io"
	"time"
)

// Default tunables for the signer and verifier.
const (
	// DefaultIssuer is the empty iss claim; deployments override it with
	// [WithSignerIssuer] or [WithVerifierIssuer].
	DefaultIssuer = ""

	// DefaultLeeway is the clock-skew tolerance applied to exp / nbf / iat
	// verification to absorb drift between signer and verifier.
	DefaultLeeway = 30 * time.Second

	// MaxLeeway caps the clock-skew tolerance. A larger [WithLeeway] value is
	// clamped to it, so a configuration typo such as 24h cannot extend every
	// token's lifetime by a day.
	MaxLeeway = 5 * time.Minute

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

type signerOptions struct {
	issuer string `opt:"SignerIssuer" optgen:"default=DefaultIssuer"`
	// WithSignerAllowedAlgorithms replaces the signer's algorithm allow-list.
	// An empty list keeps the current algorithms.
	allowedAlgorithms []Algorithm   `opt:"SignerAllowedAlgorithms" optval:"clone,notrim,nodup" optgen:"default=DefaultAllowedAlgorithms()"`
	rand              io.Reader     `optgen:"default=defaultRand"`
	clock             Clock         `opt:"SignerClock" optgen:"default=defaultClock"`
	maxTokenLifetime  time.Duration `optgen:"default=DefaultMaxTokenLifetime"`
	// tokenType, when non-empty, is written to the signed token's typ header by
	// [Signer.Sign] (for example [TypeAccessToken] / "at+jwt" per RFC 9068).
	// Empty leaves the golang-jwt default of "JWT". Set via [WithTokenType].
	tokenType string `optgen:"default=DefaultTokenType"`
	// allowEmptyKeyID makes [Signer.Sign] accept a [SigningKey] with an empty
	// KeyID and omit the kid header instead of returning [ErrSigningKeyInvalid].
	// Use it only when the receiving party does not resolve keys by kid, such as
	// an OAuth2 JWT-bearer assertion whose key is bound to its issuer (RFC 7523).
	// Set via [WithAllowEmptyKeyID].
	allowEmptyKeyID bool
}

type verifierOptions struct {
	issuer string `opt:"VerifierIssuer" optgen:"default=DefaultIssuer"`
	// WithVerifierAllowedAlgorithms replaces the verifier's algorithm allow-list.
	// An empty list keeps the current algorithms.
	allowedAlgorithms []Algorithm   `opt:"VerifierAllowedAlgorithms" optval:"clone,notrim,nodup" optgen:"default=DefaultAllowedAlgorithms()"`
	clock             Clock         `opt:"VerifierClock" optgen:"default=defaultClock"`
	leeway            time.Duration `optgen:"default=DefaultLeeway" optval:"positive=allow_zero"`
	// WithExpirationRequired makes an exp claim mandatory on verify.
	expirationRequired bool `optgen:"default=DefaultExpirationRequired"`
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
	// WithAudiences sets the acceptable aud values. Verification accepts a token
	// whose aud claim intersects this set; an empty set leaves aud unchecked. A
	// call with no audiences is ignored.
	audiences []string `opt:"Audiences" optval:"clone,notrim,nodup"`
	// WithRequiredClaims names claims that must be present, beyond the temporal
	// claims the verifier already enforces. Verification fails with [ErrClaimMissing]
	// when any is absent. A call with no claims is ignored.
	requiredClaims []string `opt:"RequiredClaims" optval:"clone,notrim,nodup"`
	// revocation, when set, rejects a verified token whose jti has been revoked,
	// after its signature and registered claims validate, with [ErrTokenRevoked].
	// A token without a string jti is rejected with [ErrClaimMissing], because it
	// could never be revoked; a checker that cannot answer rejects it with
	// [ErrRevocationUnavailable]. Nil leaves revocation unchecked. Back it with
	// auth/denylist or any [RevocationChecker]. Set via the generated
	// WithRevocation.
	revocation RevocationChecker `optgen:"notnil"`
	// revocationAllowMissingID accepts a token without a jti even though a
	// revocation checker is configured; such a token can never be revoked. It
	// is written by the hand-written [WithInsecureRevocationAllowMissingID],
	// whose name states the risk and which takes no argument.
	revocationAllowMissingID bool `optgen:"manual"`
}

// WithInsecureRevocationAllowMissingID lets the verifier accept a token that
// carries no jti although [WithRevocation] is configured. Such a token can never
// be revoked, so leave this off unless every token source is trusted to omit
// the jti and revocation is not meant to cover it.
func WithInsecureRevocationAllowMissingID() VerifierOption {
	return func(o *verifierOptions) {
		o.revocationAllowMissingID = true
	}
}

// WithExpirationOptional makes the exp claim optional, reversing the
// fail-closed default ([DefaultExpirationRequired]) under which a token without
// exp is rejected. When optional, exp is still validated if present but a token
// omitting it is accepted. It is the inverse of [WithExpirationRequired]; the
// last of the two applied wins. Use it for token profiles that legitimately
// omit exp (for example a signed userinfo response).
func WithExpirationOptional() VerifierOption {
	return func(o *verifierOptions) {
		o.expirationRequired = false
	}
}
