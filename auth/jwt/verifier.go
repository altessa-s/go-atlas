// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jwt

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// Verifier validates JWTs and returns the verified [Claims]. It resolves the
// verification key through a [KeyResolver], enforces the algorithm allow-list
// before checking the signature, and validates the temporal, issuer, subject,
// audience, and required claims. Verification is fail-closed.
type Verifier struct {
	resolver KeyResolver
	opts     *options
	// parserOpts are the golang-jwt parser options, fixed for the verifier's
	// lifetime and built once in NewVerifier so verification allocates no option
	// slice per call.
	parserOpts []gojwt.ParserOption
}

// NewVerifier builds a Verifier that resolves keys through resolver.
func NewVerifier(resolver KeyResolver, opts ...Option) *Verifier {
	o := newOptions(opts...)
	return &Verifier{resolver: resolver, opts: o, parserOpts: buildParserOptions(o)}
}

// buildParserOptions translates the verifier options into golang-jwt parser
// options for the registered-claim checks golang-jwt performs (valid methods,
// leeway, clock, exp/nbf-required, iat, issuer, subject). Audience and required
// claims are enforced by the package on top of these, so they are not included.
func buildParserOptions(o *options) []gojwt.ParserOption {
	parserOpts := []gojwt.ParserOption{
		gojwt.WithValidMethods(methodNames(o.allowedAlgorithms)),
		gojwt.WithLeeway(o.leeway),
		gojwt.WithTimeFunc(o.clock.Now),
	}
	if o.expirationRequired {
		parserOpts = append(parserOpts, gojwt.WithExpirationRequired())
	}
	if o.notBeforeRequired {
		parserOpts = append(parserOpts, gojwt.WithNotBeforeRequired())
	}
	if o.issuedAt {
		parserOpts = append(parserOpts, gojwt.WithIssuedAt())
	}
	if o.issuer != "" {
		parserOpts = append(parserOpts, gojwt.WithIssuer(o.issuer))
	}
	if o.subject != "" {
		parserOpts = append(parserOpts, gojwt.WithSubject(o.subject))
	}
	return parserOpts
}

// Verify parses raw (allowed algorithms only), checks its signature against the
// key the resolver returns for the token, validates the temporal, issuer, and
// subject claims with the configured clock-skew leeway, and enforces the
// configured audience, required claims, and typ. It returns the verified
// [Claims].
//
// Errors map onto the package sentinels: [ErrTokenExpired] past exp,
// [ErrTokenTypeInvalid] for a typ mismatch, [ErrClaimMissing] for an absent
// required claim, and [ErrTokenInvalid] for parse, algorithm, signature,
// issuer, subject, or audience failures. An error returned by the [KeyResolver]
// propagates unwrapped so a caller can translate it.
func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	claims, _, err := v.VerifyWithHeader(ctx, raw)
	return claims, err
}

// VerifyWithHeader is like [Verifier.Verify] but also returns the token's
// verified header (alg / kid / typ), which a caller needs to act on
// header-derived data such as a kid for a revocation lookup. On error the
// returned header is the zero value.
func (v *Verifier) VerifyWithHeader(ctx context.Context, raw string) (Claims, Header, error) {
	claims, hdr, err := v.parse(ctx, raw, v.parserOpts)
	if err != nil {
		return nil, Header{}, err
	}
	if v.opts.expectedTokenType != "" && !tokenTypeMatches(hdr.Typ, v.opts.expectedTokenType) {
		return nil, Header{}, fmt.Errorf("%w: %q", ErrTokenTypeInvalid, hdr.Typ)
	}
	if err := v.validateExtraClaims(claims); err != nil {
		return nil, Header{}, err
	}
	return claims, hdr, nil
}

// VerifySignature verifies raw's signature and algorithm (the allow-list and the
// resolved key's algorithm binding) but skips all claim validation — temporal,
// issuer, subject, audience, typ, and required claims. It returns the parsed
// claims and verified header so a caller can act on the claims before validating
// them (for example to select a validation profile from otherwise-unverified
// claims). The claims MUST NOT be trusted until validated; pass them to
// [Verifier.ValidateClaims], or use [Verifier.Verify] for the full pipeline.
func (v *Verifier) VerifySignature(ctx context.Context, raw string) (Claims, Header, error) {
	// Build a fresh slice so the appended option never mutates the shared
	// v.parserOpts backing array.
	opts := make([]gojwt.ParserOption, 0, len(v.parserOpts)+1)
	opts = append(opts, v.parserOpts...)
	opts = append(opts, gojwt.WithoutClaimsValidation())
	return v.parse(ctx, raw, opts)
}

// ValidateClaims validates an already-parsed claim set against the verifier's
// registered-claim policy — temporal (exp / nbf / iat with leeway), issuer,
// subject, and the exp/nbf-required flags — plus the configured required claims
// and audience. It performs no signature check, so it suits validating claims a
// caller obtained from [Verifier.VerifySignature]. typ is not checked here as it
// is a header, not a claim.
//
// The algorithm allow-list is NOT re-applied here — it is a signature-time
// check enforced by [Verifier.Verify] / [Verifier.VerifySignature]. Call this
// only on claims whose signature you have already verified.
//
// Errors map onto the same sentinels as [Verifier.Verify].
func (v *Verifier) ValidateClaims(claims Claims) error {
	if err := gojwt.NewValidator(v.parserOpts...).Validate(gojwt.MapClaims(claims)); err != nil {
		if errors.Is(err, gojwt.ErrTokenExpired) {
			return fmt.Errorf("%w: %w", ErrTokenExpired, err)
		}
		return fmt.Errorf("%w: %w", ErrTokenInvalid, err)
	}
	return v.validateExtraClaims(claims)
}

// RevocationChecker reports whether a token, identified by its jti claim, has
// been revoked. It is the consumer-side view of a denylist — auth/denylist
// satisfies it — declared here so the verifier depends on the seam, not a
// concrete store, and can be backed by an in-memory denylist now and a
// distributed one later.
type RevocationChecker interface {
	// IsRevoked reports whether the token id (jti) has been revoked.
	IsRevoked(id string) bool
}

// validateExtraClaims enforces the checks the package layers on top of
// golang-jwt's registered-claim validation: the required claims, the accepted
// audience set, and revocation of the token's jti.
func (v *Verifier) validateExtraClaims(claims Claims) error {
	for _, name := range v.opts.requiredClaims {
		if !claims.Has(name) {
			return fmt.Errorf("%w: %q", ErrClaimMissing, name)
		}
	}
	if len(v.opts.audiences) > 0 && !audienceMatches(claims.Audience(), v.opts.audiences) {
		return fmt.Errorf("%w: audience not accepted", ErrTokenInvalid)
	}
	if v.opts.revocation != nil {
		if jti := claims.ID(); jti != "" && v.opts.revocation.IsRevoked(jti) {
			return fmt.Errorf("%w: %q", ErrTokenRevoked, jti)
		}
	}
	return nil
}

// parse runs the golang-jwt parse with the resolver key func and the given
// parser options, returning the claims and verified header. A key-resolution
// failure propagates unwrapped; temporal and parse failures map onto
// [ErrTokenExpired] / [ErrTokenInvalid].
func (v *Verifier) parse(ctx context.Context, raw string, parserOpts []gojwt.ParserOption) (Claims, Header, error) {
	// resolveErr captures a key-resolution failure raised inside the key func so
	// the precise error survives golang-jwt's error wrapping.
	var resolveErr error
	var hdr Header
	claims := gojwt.MapClaims{}
	_, err := gojwt.ParseWithClaims(raw, claims, func(t *gojwt.Token) (any, error) {
		hdr = Header{Alg: t.Method.Alg()}
		if kid, ok := t.Header["kid"].(string); ok {
			hdr.Kid = kid
		}
		if typ, ok := t.Header["typ"].(string); ok {
			hdr.Typ = typ
		}
		// Hand the resolver a clone of the still-unverified claims: it must not
		// be able to mutate the map that parse returns as the verified result.
		vk, e := v.resolver.ResolveKey(ctx, hdr, Claims(maps.Clone(claims)))
		if e != nil {
			resolveErr = e
			return nil, e
		}
		// A resolved key may bind itself to one algorithm; when it does, the
		// token header must match so a key cannot be coerced into verifying a
		// different scheme.
		if vk.Algorithm != "" && vk.Algorithm.String() != t.Method.Alg() {
			resolveErr = ErrAlgorithmNotAllowed
			return nil, resolveErr
		}
		if vk.Key == nil {
			// The key is absent, not the algorithm: surface ErrTokenInvalid so a
			// caller does not misread a missing key as an algorithm rejection.
			resolveErr = fmt.Errorf("%w: resolver returned no verification key", ErrTokenInvalid)
			return nil, resolveErr
		}
		return vk.Key, nil
	}, parserOpts...)
	if err != nil {
		if resolveErr != nil {
			return nil, Header{}, resolveErr
		}
		if errors.Is(err, gojwt.ErrTokenExpired) {
			return nil, Header{}, fmt.Errorf("%w: %w", ErrTokenExpired, err)
		}
		return nil, Header{}, fmt.Errorf("%w: %w", ErrTokenInvalid, err)
	}
	return Claims(claims), hdr, nil
}

// tokenTypeMatches reports whether the token's typ header satisfies the expected
// type. JWT typ values are media types, so the comparison is case-insensitive
// and tolerates an optional "application/" prefix on either side (RFC 9068
// allows both "at+jwt" and "application/at+jwt").
func tokenTypeMatches(got, want string) bool {
	norm := func(s string) string {
		return strings.TrimPrefix(strings.ToLower(s), "application/")
	}
	return norm(got) == norm(want)
}

// audienceMatches reports whether the token's audiences intersect the accepted
// set.
func audienceMatches(tokenAud, accepted []string) bool {
	for _, a := range tokenAud {
		if slices.Contains(accepted, a) {
			return true
		}
	}
	return false
}
